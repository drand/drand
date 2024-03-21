package common

import (
	"testing"
)

var (
	version123 = Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: "",
	}

	version124 = Version{
		Major:      1,
		Minor:      2,
		Patch:      4,
		Prerelease: "",
	}

	version123pre = Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: "pre",
	}

	version130 = Version{
		Major:      1,
		Minor:      3,
		Patch:      0,
		Prerelease: "",
	}

	version130pre = Version{
		Major:      1,
		Minor:      3,
		Patch:      0,
		Prerelease: "pre",
	}

	version157 = Version{
		Major:      1,
		Minor:      5,
		Patch:      7,
		Prerelease: "",
	}

	version158 = Version{
		Major:      1,
		Minor:      5,
		Patch:      8,
		Prerelease: "",
	}

	version200 = Version{
		Major:      2,
		Minor:      0,
		Patch:      0,
		Prerelease: "",
	}
)

func TestVersionStringNoPre(t *testing.T) {
	actual := version123.String()
	expected := "1.2.3"

	if actual != expected {
		t.Fatalf("Incorrect version string. Actual: %s, expected: %s", actual, expected)
	}
}

func TestVersionStringPre(t *testing.T) {
	actual := version123pre.String()
	expected := "1.2.3-pre"

	if actual != expected {
		t.Fatalf("Incorrect version string. Actual: %s, expected: %s", actual, expected)
	}
}

func TestVersionCompatible(tm *testing.T) {
	testCompatible := func(t *testing.T, a Version, b Version) {
		if !a.IsCompatible(b) || !b.IsCompatible(a) {
			t.Fatalf("Version %s should be compatible with %s", a, b)
		}
	}

	testIncompatible := func(t *testing.T, a Version, b Version) {
		if a.IsCompatible(b) {
			t.Fatalf("Version %s should not be compatible with %s", a, b)
		}

		if b.IsCompatible(a) {
			t.Fatalf("Version %s should not be compatible with %s", b, a)
		}
	}

	for _, tt := range []struct {
		a        Version
		b        Version
		isCompat bool
	}{
		{version123, version123pre, true},
		{version123, version124, true},
		{version157, version158, true},
		{version158, version158, true},
		{version158, version200, true},
		{version123, version157, false},
		{version123, version130pre, true},
		{version123, version200, false},
		{version157, version200, false},
		{version123pre, version130pre, true},
		{version123pre, version130, true},
		{version123pre, version157, false},
	} {
		compat := tt.isCompat
		a := tt.a
		b := tt.b

		tm.Run("normal", func(t *testing.T) {
			if compat {
				testCompatible(t, a, b)
			} else {
				testIncompatible(t, a, b)
			}
		})

		tm.Run("forced", func(t *testing.T) {
			t.Setenv("DISABLE_VERSION_CHECK", "1")
			testCompatible(t, a, b)
		})
	}
}
