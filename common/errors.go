package common

import "errors"

// ErrNotPartOfGroup indicates that this node is not part of the group for a specific beacon ID
var ErrNotPartOfGroup = errors.New("this node is not part of the group")

// ErrPeerNotFound indicates that a peer is not part of any group that this node knows of
var ErrPeerNotFound = errors.New("peer not found")

// ErrInvalidChainHash means there was an error or a mismatch with the chain hash
var ErrInvalidChainHash = errors.New("incorrect chain hash")

var ErrEmptyClientUnsupportedGet = errors.New("not supported")
