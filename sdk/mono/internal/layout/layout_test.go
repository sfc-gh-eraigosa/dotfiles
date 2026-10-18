package layout

import (
	"fmt"
	"testing"
)

func sum(v []int) int {
	s := 0
	for _, x := range v {
		s += x
	}
	return s
}

func TestRowsDefaultLayout(t *testing.T) {
	// title 5%, htop 60%, nvtop 30%, gsl 5%.
	cases := []struct {
		height int
		want   string
	}{
		{24, "[1 15 7 1]"},
		{40, "[2 24 12 2]"},
		{60, "[3 36 18 3]"},
		{80, "[4 48 24 4]"},
		{10, "[1 5 3 1]"}, // the gsl minimum row comes out of the largest pane
	}
	for _, tc := range cases {
		got := Rows(tc.height, []int{5, 60, 30, 5})
		if fmt.Sprint(got) != tc.want {
			t.Errorf("Rows(%d) = %v, want %s", tc.height, got, tc.want)
		}
		if sum(got) != tc.height {
			t.Errorf("Rows(%d) sums to %d", tc.height, sum(got))
		}
	}
}

func TestRowsDroppedPaneSharesGoToTheRest(t *testing.T) {
	// nvtop dropped (WSL): htop and gsl split the panes' 95% as 60:5.
	got := Rows(40, []int{5, 60, 5})
	if fmt.Sprint(got) != "[2 35 3]" {
		t.Fatalf("Rows(40, htop+gsl) = %v, want [2 35 3]", got)
	}
	if sum(got) != 40 {
		t.Fatalf("sum = %d", sum(got))
	}
}

func TestRowsMinimumOneRow(t *testing.T) {
	got := Rows(8, []int{5, 90, 5, 1})
	for i, v := range got {
		if v < 1 {
			t.Fatalf("region %d got %d rows: %v", i, v, got)
		}
	}
	if sum(got) != 8 {
		t.Fatalf("sum = %d: %v", sum(got), got)
	}
}

func TestRowsTinyAndDegenerate(t *testing.T) {
	if got := Rows(2, []int{5, 60, 30, 5}); fmt.Sprint(got) != "[1 1 0 0]" {
		t.Fatalf("Rows(2) = %v", got)
	}
	if got := Rows(0, []int{5, 95}); sum(got) != 0 {
		t.Fatalf("Rows(0) = %v", got)
	}
	if got := Rows(10, nil); len(got) != 0 {
		t.Fatalf("Rows(10, nil) = %v", got)
	}
	if got := Rows(10, []int{5}); fmt.Sprint(got) != "[10]" {
		t.Fatalf("title only = %v", got)
	}
}

func TestRowsAlwaysSumsToHeight(t *testing.T) {
	sets := [][]int{{5, 60, 30, 5}, {5, 60, 5}, {10, 33, 33, 34}, {1, 1, 1, 1, 1}, {50, 100}, {5, 200, 1}}
	for _, p := range sets {
		for h := len(p); h <= 120; h++ {
			got := Rows(h, p)
			if sum(got) != h {
				t.Fatalf("Rows(%d, %v) = %v sums to %d", h, p, got, sum(got))
			}
			for i, v := range got {
				if v < 1 {
					t.Fatalf("Rows(%d, %v)[%d] = 0", h, p, i)
				}
			}
		}
	}
}
