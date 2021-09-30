package common

func NewContext(version *NodeVersion) *Context {
	return &Context{NodeVersion: version}
}
