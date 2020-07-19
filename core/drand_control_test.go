package core

import (
	"testing"
	"time"

	"github.com/drand/drand/key"
	"github.com/drand/drand/log"
	"github.com/drand/kyber"
	"github.com/drand/kyber/util/random"
	clock "github.com/jonboulle/clockwork"
)

func TestValidateGroupTransitionGenesisTime(t *testing.T) {
	d := Drand{log: log.DefaultLogger()}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{GenesisTime: 0}
	newgrp = key.Group{GenesisTime: 1}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	if err == nil {
		t.Fatal("expected error validating group genesis time")
	}
	if err.Error() != "control: old and new group have different genesis time" {
		t.Fatal("unexpected validation error", err)
	}
}

func TestValidateGroupTransitionPeriod(t *testing.T) {
	d := Drand{log: log.DefaultLogger()}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{Period: 10}
	newgrp = key.Group{Period: 20}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	if err == nil {
		t.Fatal("expected error validating group period")
	}
	if err.Error() != "control: old and new group have different period - unsupported feature at the moment" {
		t.Fatal("unexpected validation error", err)
	}
}

func TestValidateGroupTransitionGenesisSeed(t *testing.T) {
	d := Drand{log: log.DefaultLogger()}
	var oldgrp, newgrp key.Group

	randomDistPublic := func(n int) *key.DistPublic {
		publics := make([]kyber.Point, n)
		for i := range publics {
			k := key.KeyGroup.Scalar().Pick(random.New())
			publics[i] = key.KeyGroup.Point().Mul(k, nil)
		}
		return &key.DistPublic{Coefficients: publics}
	}

	oldgrp = key.Group{PublicKey: randomDistPublic(4)}
	newgrp = key.Group{PublicKey: randomDistPublic(3)}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	if err == nil {
		t.Fatal("expected error validating group genesis seed")
	}
	if err.Error() != "control: old and new group have different genesis seed" {
		t.Fatal("unexpected validation error", err)
	}
}

func TestValidateGroupTransitionTime(t *testing.T) {
	d := Drand{
		log:  log.DefaultLogger(),
		opts: &Config{clock: clock.NewRealClock()},
	}
	var oldgrp, newgrp key.Group

	oldgrp = key.Group{TransitionTime: time.Now().Unix()}
	newgrp = key.Group{TransitionTime: time.Now().Unix() - 1, GenesisSeed: oldgrp.GetGenesisSeed()}

	err := d.validateGroupTransition(&oldgrp, &newgrp)
	if err == nil {
		t.Fatal("expected error validating group period")
	}
	if err.Error() != "control: new group with transition time in the past" {
		t.Fatal("unexpected validation error", err)
	}
}
