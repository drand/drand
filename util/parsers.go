package util

import (
	"bytes"

	"github.com/BurntSushi/toml"
	"github.com/drand/drand/key"
)

func ParseGroupFileBytes(groupFileBytes []byte) (*key.Group, error) {
	//nolint:nilnil
	if len(groupFileBytes) == 0 {
		return nil, nil
	}

	t := key.GroupTOML{}
	_, err := toml.NewDecoder(bytes.NewReader(groupFileBytes)).Decode(&t)
	if err != nil {
		return nil, err
	}
	previousGroupFile := key.Group{}
	err = previousGroupFile.FromTOML(&t)
	if err != nil {
		return nil, err
	}
	return &previousGroupFile, nil
}
