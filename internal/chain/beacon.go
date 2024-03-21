package chain

import (
	"context"
)

type previousBeaconNeeded bool

var requiresPreviousBeacon previousBeaconNeeded = true

func SetPreviousRequiredOnContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, requiresPreviousBeacon, requiresPreviousBeacon)
}

func PreviousRequiredFromContext(ctx context.Context) bool {
	_, ok := ctx.Value(requiresPreviousBeacon).(previousBeaconNeeded)
	return ok
}
