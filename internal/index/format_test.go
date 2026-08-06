package index

import "testing"

func TestHashStrDeterministic(t *testing.T) {
	samples := []string{"", "Ah01g000200.1", "arahy.Tifrunner.gnm2.ann2.Ah01g000200", "g1.t1.exon1", "Glyma.16G009500.2.Wm82.a4.v1", ">", "ID=foo;Parent=bar"}
	for _, s := range samples {
		if h1, h2 := HashStr(s), HashStr(s); h1 != h2 {
			t.Errorf("HashStr(%q) not deterministic: %d != %d", s, h1, h2)
		}
	}
}

// Hash value 0 is reserved as the empty-slot sentinel in the hash table,
// so HashStr must never return 0 for any input.
func TestHashStrNeverZero(t *testing.T) {
	samples := []string{"", "a", "Ah01g000200", "g2.t1.CDS3", "Glyma.U007800", "gene-1", "mRNA 1", "exon-1;cds-1"}
	for _, s := range samples {
		if h := HashStr(s); h == 0 {
			t.Errorf("HashStr(%q) returned 0 (reserved sentinel)", s)
		}
	}
}

func TestHashNonZero(t *testing.T) {
	cases := []struct {
		in   uint64
		want uint64
	}{
		{0, 1},
		{1, 1},
		{2, 2},
		{0xFFFFFFFFFFFFFFFF, 0xFFFFFFFFFFFFFFFF},
	}
	for _, c := range cases {
		if got := hashNonZero(c.in); got != c.want {
			t.Errorf("hashNonZero(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
