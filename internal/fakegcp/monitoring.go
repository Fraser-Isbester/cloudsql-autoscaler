package fakegcp

import (
	"context"
	"net"
	"regexp"
	"sync"
	"time"

	"cloud.google.com/go/monitoring/apiv3/v2/monitoringpb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Metric types the autoscaler queries.
const (
	MetricCPU         = "cloudsql.googleapis.com/database/cpu/utilization"
	MetricMemory      = "cloudsql.googleapis.com/database/memory/utilization"
	MetricMemoryBytes = "cloudsql.googleapis.com/database/memory/usage"
	MetricBackends    = "cloudsql.googleapis.com/database/postgresql/num_backends"
)

// Point is a single sample. Int, when set, is sent as an INT64 value instead of DOUBLE.
type Point struct {
	At    time.Time
	Value float64
	Int   bool
}

// Monitoring is a fake Cloud Monitoring MetricService served over real gRPC on loopback.
type Monitoring struct {
	monitoringpb.UnimplementedMetricServiceServer

	srv *grpc.Server
	lis net.Listener

	mu       sync.Mutex
	series   map[string][]Point    // metric type -> points
	errs     map[string]codes.Code // metric type -> error to return
	requests []*monitoringpb.ListTimeSeriesRequest
}

// NewMonitoring starts the fake. Call Close when done.
func NewMonitoring() (*Monitoring, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	f := &Monitoring{
		srv:    grpc.NewServer(),
		lis:    lis,
		series: map[string][]Point{},
		errs:   map[string]codes.Code{},
	}
	monitoringpb.RegisterMetricServiceServer(f.srv, f)
	go func() { _ = f.srv.Serve(lis) }()
	return f, nil
}

// Close stops the server.
func (f *Monitoring) Close() { f.srv.Stop() }

// ClientOptions returns options that point monitoring.NewMetricClient at the fake.
func (f *Monitoring) ClientOptions() []option.ClientOption {
	return []option.ClientOption{
		option.WithEndpoint(f.lis.Addr().String()),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
}

// SetSeries sets the samples returned for a metric type.
func (f *Monitoring) SetSeries(metricType string, points []Point) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.series[metricType] = points
}

// FailMetric makes queries for metricType fail with code.
func (f *Monitoring) FailMetric(metricType string, code codes.Code) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[metricType] = code
}

// Requests returns all ListTimeSeries requests received.
func (f *Monitoring) Requests() []*monitoringpb.ListTimeSeriesRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]*monitoringpb.ListTimeSeriesRequest(nil), f.requests...)
}

var metricTypeRe = regexp.MustCompile(`metric\.type="([^"]+)"`)

// ListTimeSeries implements monitoringpb.MetricServiceServer.
func (f *Monitoring) ListTimeSeries(_ context.Context, req *monitoringpb.ListTimeSeriesRequest) (*monitoringpb.ListTimeSeriesResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)

	m := metricTypeRe.FindStringSubmatch(req.GetFilter())
	if m == nil {
		return nil, status.Error(codes.InvalidArgument, "filter has no metric.type")
	}
	if code, ok := f.errs[m[1]]; ok {
		return nil, status.Error(code, "fake failure")
	}
	pts := f.series[m[1]]
	if len(pts) == 0 {
		return &monitoringpb.ListTimeSeriesResponse{}, nil
	}
	ts := &monitoringpb.TimeSeries{}
	for _, p := range pts {
		v := &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_DoubleValue{DoubleValue: p.Value}}
		if p.Int {
			v = &monitoringpb.TypedValue{Value: &monitoringpb.TypedValue_Int64Value{Int64Value: int64(p.Value)}}
		}
		ts.Points = append(ts.Points, &monitoringpb.Point{
			Interval: &monitoringpb.TimeInterval{EndTime: timestamppb.New(p.At)},
			Value:    v,
		})
	}
	return &monitoringpb.ListTimeSeriesResponse{TimeSeries: []*monitoringpb.TimeSeries{ts}}, nil
}

// Constant returns n samples at value v, spaced step apart and ending at end.
func Constant(end time.Time, step time.Duration, n int, v float64) []Point {
	pts := make([]Point, n)
	for i := range n {
		pts[i] = Point{At: end.Add(-time.Duration(n-1-i) * step), Value: v}
	}
	return pts
}
