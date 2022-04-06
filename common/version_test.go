package common

import (
	"os"
	"testing"
)

func TestVersionStringNoPre(t *testing.T) {
	var version = Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: "",
	}

	actual := version.String()
	expected := "1.2.3"

	if actual != expected {
		t.Fatalf("Incorrect version string. Actual: %s, expected: %s", actual, expected)
	}
}

func TestVersionStringPre(t *testing.T) {
	version := Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: "pre",
	}

	actual := version.String()
	expected := "1.2.3+pre"

	if actual != expected {
		t.Fatalf("Incorrect version string. Actual: %s, expected: %s", actual, expected)
	}
}

func TestVersionCompatible(t *testing.T) {
	version000 := Version{
		Major:      0,
		Minor:      0,
		Patch:      0,
		Prerelease: "",
	}

	version123 := Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: "",
	}

	version124 := Version{
		Major:      1,
		Minor:      2,
		Patch:      4,
		Prerelease: "",
	}

	version123pre := Version{
		Major:      1,
		Minor:      2,
		Patch:      3,
		Prerelease: "+pre",
	}

	version130 := Version{
		Major:      1,
		Minor:      3,
		Patch:      0,
		Prerelease: "",
	}

	version130pre := Version{
		Major:      1,
		Minor:      3,
		Patch:      0,
		Prerelease: "pre",
	}

	version200 := Version{
		Major:      2,
		Minor:      0,
		Patch:      0,
		Prerelease: "",
	}

	testCompatible := func(a Version, b Version) {
		if !a.IsCompatible(b) || !b.IsCompatible(a) {
			t.Fatalf("Version %s should be compatible with %s", a, b)
		}
	}

	testIncompatible := func(a Version, b Version) {
		if a.IsCompatible(b) || b.IsCompatible(a) {
			t.Fatalf("Version %s should not be compatible with %s", a, b)
		}
	}

	testCompatible(version123, version123)
	testCompatible(version123, version123pre)
	testCompatible(version123, version124)

	testIncompatible(version123, version130)
	testIncompatible(version123, version130pre)
	testIncompatible(version123, version200)
	testIncompatible(version123pre, version130pre)

	os.Setenv("DISABLE_VERSION_CHECK", "1")
	testCompatible(version123, version000)
	testCompatible(version123pre, version000)
	testCompatible(version124, version000)
	testCompatible(version130, version000)
	testCompatible(version130pre, version000)
	testCompatible(version200, version000)
	testCompatible(version123, version130)
	testCompatible(version123, version130pre)
	testCompatible(version123, version200)
	testCompatible(version123pre, version130pre)

}
