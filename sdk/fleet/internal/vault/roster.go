package vault

import "time"

// Managed returns the Public of every pending or active entry, never retired.
func (r Roster) Managed() []string {
	var out []string
	for _, e := range r.Entries {
		if e.State == KeyPending || e.State == KeyActive {
			out = append(out, e.Public)
		}
	}
	return out
}

// Add appends a new pending entry.
func (r *Roster) Add(ws, pub, fp string, now time.Time) {
	r.Entries = append(r.Entries, RosterEntry{Workstation: ws, Public: pub, Fingerprint: fp, State: KeyPending, Changed: now})
}

// Verified records that fp's key was seen working on host (deduplicated).
func (r *Roster) Verified(fp, host string) {
	for i := range r.Entries {
		if r.Entries[i].Fingerprint != fp {
			continue
		}
		for _, h := range r.Entries[i].VerifiedOn {
			if h == host {
				return
			}
		}
		r.Entries[i].VerifiedOn = append(r.Entries[i].VerifiedOn, host)
		return
	}
}

// Activate makes fp active and retires the same workstation's previously
// active entries, returning them. Unknown fp is a no-op.
func (r *Roster) Activate(fp string, now time.Time) (retired []RosterEntry) {
	idx := -1
	for i, e := range r.Entries {
		if e.Fingerprint == fp {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	ws := r.Entries[idx].Workstation
	for i := range r.Entries {
		if i != idx && r.Entries[i].Workstation == ws && r.Entries[i].State == KeyActive {
			r.Entries[i].State = KeyRetired
			r.Entries[i].Changed = now
			retired = append(retired, r.Entries[i])
		}
	}
	if r.Entries[idx].State != KeyActive {
		r.Entries[idx].State = KeyActive
		r.Entries[idx].Changed = now
	}
	return retired
}

// RetirableOn reports whether oldFP is retired and its workstation's active
// key has been verified on host.
func (r Roster) RetirableOn(oldFP, host string) bool {
	var old *RosterEntry
	for i := range r.Entries {
		if r.Entries[i].Fingerprint == oldFP {
			old = &r.Entries[i]
			break
		}
	}
	if old == nil || old.State != KeyRetired {
		return false
	}
	for _, e := range r.Entries {
		if e.Workstation == old.Workstation && e.State == KeyActive {
			for _, h := range e.VerifiedOn {
				if h == host {
					return true
				}
			}
		}
	}
	return false
}
