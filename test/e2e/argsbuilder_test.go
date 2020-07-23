package e2e

func generateKeypairArgs(conf Config) []string {
	return []string{"generate-keypair", "--folder", conf.folder, host + ":" + conf.ports.priv}
}

func startWithTLSArgs(conf Config, certsDir string) []string {
	return []string{
		"start",
		"--tls-cert", conf.tls.certpath,
		"--tls-key", conf.tls.keypath,
		"--certs-dir", certsDir,
		"--folder", conf.folder,
		"--private-listen", host + ":" + conf.ports.priv,
		"--control", conf.ports.ctl,
		"--public-listen", host + ":" + conf.ports.pub,
	}
}

func shareParticipantArgs(conf Config, leaderConf Config, secret string) []string {
	return []string{
		"share",
		"--control", conf.ports.ctl,
		"--connect", host + ":" + leaderConf.ports.priv,
		"--secret", secret,
	}
}
