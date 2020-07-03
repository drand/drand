# Release Process

This document describes the process for a public drand release.

## Release Philosophy

Drand aims to be a scalable and robust implementation of the Drand protocol, augmented with the designed mechanism so that it deliveres a scalable service to its users. The definition of success is a bug free implementation that serves its core purpose: deliver unbiasable randomness as a service. To achieve this, we will be issueing releases with bug fixes (and security fixes), new performance optimizations and/or improved APIs, however, we do not envision a constant changing drand implementation beyond v1.0.0. Once v1.0.0 is delivered, we expect drand implementation to be fairly stable.

With this in mind, drand's release philosophy does not contemplate a strict release cadence, rather, the releases are done when needed.

Additionally, drand has (at the current moment) one major large deployment, the [League of Entropy](https://www.cloudflare.com/leagueofentropy), which has both a Testnet and a Mainnet. Drand will be shipping it's releases to the Testnet first and then later graduate them to the Mainnet as they have shown to be robust in a multi-party deployment.

## Release Flow

### Major / Minor Releases

#### Tagging an RC

- [ ] A release is initiated by creating a protected release branch 'release-va.b' marking the major/minor release.
- [ ] In preparation for a release, the maintainer should look at the commit log to create a change log summarizing changes, with explicit notes of:
  - [ ] wire protocol changes that will impact group or node-client interactions.
  - [ ] protocol changes that will impact setup or resharing.
  - [ ] feature / behavior deviations.
- [ ] Once this branch is created, protected, and pushed, a signed tag should be created to indicate the first release candidate 'va.b.0-rc1'.
- [ ] This tag will be turned by the `go-releaser` CI action into a release candidate on github, and will notify anyone who is watching releases of the new action.

#### Automated Testing

- [ ] This release should go through:
  - [ ] Unit testing
  - [ ] Regression Testing
  - [ ] Testground Testing
  - [ ] Simulated Workflow (demo) Testing

#### Testing in LoE Testnet

- [ ] Reach out to the LoE partners to deploy the RC to Testnet. Ensuring that while the migration happens, there are no regressions between versions.
- [ ] A minimum 1 week 'soak time' should follow a release candidate release before the first non-release-candidate `va.b.c` tag is pushed to create a full release on the new branch.

#### Releasing and deploying to LoE Mainnet

- [ ] Reach out to LoE partners to deploy the latest release
- [ ] Releases should be communicated on the drand website through a post

### Patch Releases (aka small bugfixes)

- [ ] To fix issues on existing release branches, fix commits must first be cherry-picked to the release branch.
- [ ] A summary tag should be created, and signed explaining the fixes.
- [ ] A post should be made on the website explaining the need for the bugfix release.

### Releases with Security implications

If the fix has security implications, a smooth transition plan should be designed and communicated to LoE partners, so that we can ensure that the network is upgraded before sharing publicly of a known vulnerability on a service that users are depending on. We will handle these situations with extreme care and might be more quiet about them due to their nature. To learn how to report a vulnerability, please consult [SECURITY](./SECURITY.md)
