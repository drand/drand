package common

func NewMetadata(version *NodeVersion) *Metadata {
	return &Metadata{NodeVersion: version}
}
