package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/auth"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/config"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/dash"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/grafana"
	"github.com/sfc-gh-eraigosa/dotfiles/sdk/gdash/internal/vars"
	"github.com/spf13/cobra"
)

// QuietTimeout bounds `gdash doctor --quiet`, mono's probe, which has 3 s.
const QuietTimeout = 2 * time.Second

var (
	doctorQuiet   bool
	doctorProfile string
)

var errDoctor = errors.New("doctor: some checks failed")

type checker struct {
	out    io.Writer
	failed bool
}

func (c *checker) row(ok bool, what, detail string) {
	if !ok {
		c.failed = true
	}
	if c.out == nil {
		return
	}
	verdict := "ok  "
	if !ok {
		verdict = "FAIL"
	}
	fmt.Fprintf(c.out, "  %s %-24s %s\n", verdict, what, detail)
}

// loadProfile reads the config and picks the profile to check.
func loadProfile(name string) (config.Config, string, config.Profile, error) {
	path, err := config.Path()
	if err != nil {
		return config.Config{}, "", config.Profile{}, err
	}
	c, err := config.Load(path)
	if err != nil {
		return c, "", config.Profile{}, err
	}
	if len(c.Profiles) == 0 {
		return c, "", config.Profile{}, errors.New("no Grafana profile yet: gdash config add profile lab --url http://<grafana> --auth-file ~/.config/gdash/lab.cred")
	}
	if name == "" {
		name = c.DefaultProfile
	}
	p, ok := c.Profiles[name]
	if !ok {
		return c, "", config.Profile{}, fmt.Errorf("no profile named %q", name)
	}
	return c, name, p, nil
}

// doctor runs the checks in order — reachable, authenticated, then each
// view's dashboard and datasources — and reports whether all passed. quiet
// stops after authentication (mono's probe) and prints nothing.
func doctor(ctx context.Context, c config.Config, pname string, p config.Profile, quiet bool, out io.Writer) bool {
	ck := &checker{out: out}
	timeout := HTTPTimeout
	if quiet {
		ck.out, timeout = nil, QuietTimeout
	}
	cred, err := auth.Load(p.Auth)
	if err != nil {
		ck.row(false, "credential", err.Error())
		return false
	}
	cl := grafana.NewHTTP(p.URL, cred, timeout)
	h, err := cl.Health(ctx)
	if err != nil {
		ck.row(false, "reachable", err.Error())
		return false
	}
	ck.row(true, "reachable", fmt.Sprintf("%s (Grafana %s)", p.URL, h.Version))
	if err := cl.Whoami(ctx); err != nil {
		ck.row(false, "authenticated", err.Error())
		return false
	}
	ck.row(true, "authenticated", "profile "+pname)
	if quiet {
		return true
	}
	ds, err := cl.Datasources(ctx)
	if err != nil {
		ck.row(false, "datasources", err.Error())
		return false
	}
	used := map[string]grafana.DSRef{}
	names := make([]string, 0, len(c.Views))
	for n := range c.Views {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v, vp, err := c.Resolve(n)
		if err != nil || vp.URL != p.URL {
			continue // a view on another profile
		}
		uid, err := grafana.Resolve(ctx, cl, v.Dashboard)
		if err != nil {
			ck.row(false, "view "+n, err.Error()+" — gdash list")
			continue
		}
		raw, err := cl.Dashboard(ctx, uid)
		if err != nil {
			ck.row(false, "view "+n, err.Error())
			continue
		}
		d, err := dash.Parse(raw)
		if err != nil {
			ck.row(false, "view "+n, err.Error())
			continue
		}
		ck.row(true, "view "+n, fmt.Sprintf("dashboard %q (%s)", d.Title, uid))
		env := vars.New(d.Vars, v.Vars, ds, time.Hour, 120)
		for _, pn := range d.Panels {
			for _, t := range pn.Targets {
				if r, err := env.Datasource(t.Datasource); err == nil && r.Type == "prometheus" {
					used[r.UID] = r
				}
			}
		}
	}
	dsName := map[string]string{}
	for _, d := range ds {
		dsName[d.UID] = d.Name
	}
	uids := make([]string, 0, len(used))
	for u := range used {
		uids = append(uids, u)
	}
	sort.Strings(uids)
	for _, u := range uids {
		now := time.Now()
		res, err := cl.Query(ctx, grafana.QueryRequest{From: now.Add(-time.Minute), To: now,
			Queries: []grafana.Query{{RefID: "A", Datasource: used[u], Expr: "1", Instant: true, MaxDataPoints: 1}}})
		switch {
		case err != nil:
			ck.row(false, "datasource "+dsName[u], err.Error())
		case len(res.Errors) > 0:
			ck.row(false, "datasource "+dsName[u], strings.Join(values(res.Errors), "; "))
		default:
			ck.row(true, "datasource "+dsName[u], "answers a query")
		}
	}
	return !ck.failed
}

func values(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

var doctorCmd = &cobra.Command{
	Use:   "doctor [--profile p] [--quiet]",
	Short: "Check Grafana is reachable, the credential works, and every view's dashboard exists",
	Long: `doctor checks, in order: the Grafana answers, the credential is accepted,
each view's dashboard is found, and each datasource those dashboards use
answers a query. It exits 0 only when every check passes.

--quiet checks only that Grafana answers and the credential works, within
2 seconds, and prints nothing: it is mono's probe for the gdash pane.`,
	Args: cobra.NoArgs,
	RunE: func(c *cobra.Command, _ []string) error {
		quiet := doctorQuiet
		cfg, pname, p, err := loadProfile(doctorProfile)
		if err != nil {
			if quiet {
				c.Root().SilenceErrors = true
			}
			return err
		}
		ctx := context.Background()
		if quiet {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, QuietTimeout)
			defer cancel()
		}
		if doctor(ctx, cfg, pname, p, quiet, c.OutOrStdout()) {
			return nil
		}
		if quiet {
			c.Root().SilenceErrors = true
		}
		return errDoctor
	},
}

var listProfile string

var listCmd = &cobra.Command{
	Use:   "list [query]",
	Short: "List dashboards: uid, title, folder (the uid goes in a view)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		_, _, p, err := loadProfile(listProfile)
		if err != nil {
			return err
		}
		cred, err := auth.Load(p.Auth)
		if err != nil {
			return err
		}
		q := ""
		if len(args) == 1 {
			q = args[0]
		}
		hits, err := grafana.NewHTTP(p.URL, cred, HTTPTimeout).Search(context.Background(), q)
		if err != nil {
			return err
		}
		sort.Slice(hits, func(i, j int) bool { return hits[i].Title < hits[j].Title })
		for _, h := range hits {
			folder := h.FolderTitle
			if folder == "" {
				folder = "General"
			}
			fmt.Fprintf(c.OutOrStdout(), "%s\t%s\t%s\n", h.UID, h.Title, folder)
		}
		return nil
	},
}

func init() {
	doctorCmd.Flags().BoolVar(&doctorQuiet, "quiet", false, "reachability + credential only, 2 s, no output (mono's probe)")
	doctorCmd.Flags().StringVar(&doctorProfile, "profile", "", "profile to check (default: default_profile)")
	listCmd.Flags().StringVar(&listProfile, "profile", "", "profile to search (default: default_profile)")
	rootCmd.AddCommand(doctorCmd, listCmd)
}
