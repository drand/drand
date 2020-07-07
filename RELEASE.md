# Release Process

This document describes the process for a public drand release.

## Major / Minor Releases

* A release is initiated by creating a protected release branch 'release-va.b' marking the major/minor release.
* In preparation for a release, the maintainer should look at the commit log to create a change log summarizing changes, with explicit notes of:
  * wire protocol changes that will impact group or node-client interactions.
  * protocol changes that will impact setup or resharing.
  * feature / behavior deviations.
* Once this branch is created, protected, and pushed, a signed tag should be created to indicate the first release candidate 'va.b.0-rc1'.
* This tag will be turned by the `go-releaser` CI action into a release candidate on github, and will notify anyone who is watching releases of the new action.
* A minimum 1 week 'soak time' should follow a release candidate release before the first non-release-candidate `va.b.c` tag is pushed to create a full release on the new branch.
* Releases should be communicated on the drand website.


## Bugfix Releases

* To fix issues on existing release branches, fix commits must first be cherry-picked to the release branch.
* A summary tag should be created, and signed explaining the fixes.
* If the fix has security implications, it must be noted in the commit notes, such that the release alert provided by github to watchers can express this.
* A post should be made on the website explaining the need for the bugfix release.
