package updplan

// Order returns the plan's steps in a stable topological order: every
// step's needs precede it, and among ready steps the one with the lowest
// declaration index always goes next (Kahn's algorithm with a stable scan
// instead of a min-heap — the step count is small).
func (p Plan) Order() []Step {
	n := len(p.Steps)
	index := make(map[string]int, n)
	for i, st := range p.Steps {
		index[st.ID] = i
	}

	indegree := make([]int, n)
	dependents := make([][]int, n) // dependents[i] = indices that need step i
	for i, st := range p.Steps {
		for _, need := range st.Needs {
			j, ok := index[need]
			if !ok {
				continue // unknown need already reported by Parse's validation
			}
			indegree[i]++
			dependents[j] = append(dependents[j], i)
		}
	}

	done := make([]bool, n)
	out := make([]Step, 0, n)

	for len(out) < n {
		next := -1
		for i := 0; i < n; i++ {
			if !done[i] && indegree[i] == 0 {
				next = i
				break
			}
		}
		if next == -1 {
			// A cycle would already have been rejected by Parse; defensively
			// stop rather than loop forever.
			break
		}
		done[next] = true
		out = append(out, p.Steps[next])
		for _, dep := range dependents[next] {
			indegree[dep]--
		}
	}

	return out
}

// Dependents returns the transitive set of step ids that (directly or
// indirectly) need id, in Order() order.
func (p Plan) Dependents(id string) []string {
	direct := make(map[string][]string, len(p.Steps)) // needs -> [ids that need it]
	for _, st := range p.Steps {
		for _, need := range st.Needs {
			direct[need] = append(direct[need], st.ID)
		}
	}

	seen := map[string]bool{}
	var collect func(string)
	collect = func(cur string) {
		for _, dep := range direct[cur] {
			if !seen[dep] {
				seen[dep] = true
				collect(dep)
			}
		}
	}
	collect(id)

	out := make([]string, 0, len(seen))
	for _, st := range p.Order() {
		if seen[st.ID] {
			out = append(out, st.ID)
		}
	}
	return out
}

// LastStepUsing returns the id of the last step in Order() whose Repo ==
// repo (a sync or run step targeting that repo), used to place the
// synthesized restore step.
func (p Plan) LastStepUsing(repo string) (string, bool) {
	last := ""
	found := false
	for _, st := range p.Order() {
		if st.Repo == repo {
			last = st.ID
			found = true
		}
	}
	return last, found
}
