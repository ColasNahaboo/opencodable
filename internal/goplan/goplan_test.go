package goplan

import "testing"

func TestPrivacyTriState(t *testing.T) {
	cases := []struct {
		id     string
		trains bool
		known  bool
		label  string
		hide   bool
	}{
		{"glm-5.3", false, true, "safe", false},
		{"muse-spark-1.3-contributor", true, true, "trains", true},
		{"future-contributor", true, true, "trains", true},
		{"space-bunny", false, false, "unknown", true},
	}
	for _, c := range cases {
		trains, known := Privacy(c.id)
		if trains != c.trains || known != c.known {
			t.Errorf("Privacy(%q) = %v/%v, want %v/%v", c.id, trains, known, c.trains, c.known)
		}
		if got := PrivacyLabel(c.id); got != c.label {
			t.Errorf("PrivacyLabel(%q) = %q, want %q", c.id, got, c.label)
		}
		if got := ExcludedByPrivacy(c.id); got != c.hide {
			t.Errorf("ExcludedByPrivacy(%q) = %v, want %v", c.id, got, c.hide)
		}
	}
}

func TestSnapshotDate(t *testing.T) {
	if SnapshotDate == "" {
		t.Error("SnapshotDate is empty")
	}
}
