# Release Process

This document describes the process for a public drand release.

## Major / Minor release Process

* A release is initiated by creating a protected release branch 'release-va.b' marking the minor release.
* Once this branch is created, protected, and pushed, a signed tag should be created to indicate the first release candidate candidate 'va.b.0-rc1'
* This tag will be turned by the `go-releaser` CI action into a release candidate on github, and will notify anyone who is watching releases of the new action.
* A minimum 1 week 'soak time' should follow a release candidate release before the first non-release-candidate `va.b.c` tag is pushed to create a full release on the new branch.
