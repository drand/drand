package errors

import "errors"

// ErrNoBeaconStored is the error we get when a sync is called too early and
// there are no beacon above the requested round
var ErrNoBeaconStored = errors.New("no beacon stored above requested round")

// ErrNoBeaconSaved is the error returned when no beacon have been saved in the
// database yet.
var ErrNoBeaconSaved = errors.New("beacon not found in database")
