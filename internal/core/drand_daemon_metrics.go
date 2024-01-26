package core

import (
	"bytes"
	"context"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	"github.com/drand/drand/v2/common/tracer"
	"github.com/drand/drand/v2/internal/metrics"
	"github.com/drand/drand/v2/protobuf/drand"
)

func (dd *DrandDaemon) Metrics(ctx context.Context, _ *drand.MetricsRequest) (*drand.MetricsResponse, error) {
	_, span := tracer.NewSpan(ctx, "dd.Metrics")
	defer span.End()

	dd.log.Debugw("Responding to remote GRPC Metrics call.")

	mfs, done, err := prometheus.ToTransactionalGatherer(metrics.GroupMetrics).Gather()
	defer done()
	if err != nil {
		return nil, err
	}

	buf := new(bytes.Buffer)
	enc := expfmt.NewEncoder(buf, expfmt.FmtText)

	for _, mf := range mfs {
		err = enc.Encode(mf)
		if err != nil {
			dd.log.Errorw("error encoding MetricFamily", "mf", mf)
		}
	}
	if closer, ok := enc.(expfmt.Closer); ok {
		closer.Close()
	}

	span.RecordError(err)
	return &drand.MetricsResponse{
		Metrics: buf.Bytes(),
	}, err
}
