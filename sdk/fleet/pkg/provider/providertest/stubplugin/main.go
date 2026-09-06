// Command stubplugin is the scriptable process the protocol tests drive. It
// is a MAIN package because a library package cannot hold func main; the
// test-facing half is providertest.BuildStub, which compiles it.
//
// It knows nothing about any protocol: it reads newline-delimited input and
// answers each line with whatever the caller canned, so the same binary
// serves a handshake test, a deadline test and a framing test. The flags are
// the failure modes a consumer must survive.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"time"
)

func main() {
	reply := flag.String("reply", "", "answer every input line with this, terminated by a newline")
	halfLine := flag.String("half-line", "", "write this fragment WITHOUT a newline, then exit — a truncated frame")
	sleep := flag.Duration("sleep", 0, "sleep before answering, so a caller's deadline has to kill this process")
	exitAtOnce := flag.Bool("exit-at-once", false, "exit non-zero before answering anything")
	stderr := flag.String("stderr", "", "write this to stderr, so a consumer can prove it captured it")
	flag.Parse()

	if *stderr != "" {
		fmt.Fprintln(os.Stderr, *stderr)
	}
	if *exitAtOnce {
		os.Exit(1)
	}
	if *sleep > 0 {
		time.Sleep(*sleep)
	}
	if *halfLine != "" {
		fmt.Fprint(os.Stdout, *halfLine)
		return
	}

	// Unbuffered: a caller blocked on this reply must see it as soon as the
	// line is answered, and a test that kills the process mid-exchange must
	// not lose what was already written.
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for in.Scan() {
		if *reply != "" {
			fmt.Fprintln(os.Stdout, *reply)
		}
	}
}
