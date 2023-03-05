// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package routerrpc

import (
	context "context"
	lnrpc "github.com/SeFo-Finance/obd-go-bindings/lnrpc"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// RouterClient is the client API for Router service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RouterClient interface {
	//
	//SendPaymentV2 attempts to route a payment described by the passed
	//PaymentRequest to the final destination. The call returns a stream of
	//payment updates.
	OB_SendPaymentV2(ctx context.Context, in *SendPaymentRequest, opts ...grpc.CallOption) (Router_OB_SendPaymentV2Client, error)
	//
	//TrackPaymentV2 returns an update stream for the payment identified by the
	//payment hash.
	OB_TrackPaymentV2(ctx context.Context, in *TrackPaymentRequest, opts ...grpc.CallOption) (Router_OB_TrackPaymentV2Client, error)
	//
	//EstimateRouteFee allows callers to obtain a lower bound w.r.t how much it
	//may cost to send an HTLC to the target end destination.
	EstimateRouteFee(ctx context.Context, in *RouteFeeRequest, opts ...grpc.CallOption) (*RouteFeeResponse, error)
	// Deprecated: Do not use.
	//
	//Deprecated, use SendToRouteV2. SendToRoute attempts to make a payment via
	//the specified route. This method differs from SendPayment in that it
	//allows users to specify a full route manually. This can be used for
	//things like rebalancing, and atomic swaps. It differs from the newer
	//SendToRouteV2 in that it doesn't return the full HTLC information.
	SendToRoute(ctx context.Context, in *SendToRouteRequest, opts ...grpc.CallOption) (*SendToRouteResponse, error)
	//
	//SendToRouteV2 attempts to make a payment via the specified route. This
	//method differs from SendPayment in that it allows users to specify a full
	//route manually. This can be used for things like rebalancing, and atomic
	//swaps.
	SendToRouteV2(ctx context.Context, in *SendToRouteRequest, opts ...grpc.CallOption) (*lnrpc.HTLCAttempt, error)
	//
	//ResetMissionControl clears all mission control state and starts with a clean
	//slate.
	ResetMissionControl(ctx context.Context, in *ResetMissionControlRequest, opts ...grpc.CallOption) (*ResetMissionControlResponse, error)
	//
	//QueryMissionControl exposes the internal mission control state to callers.
	//It is a development feature.
	QueryMissionControl(ctx context.Context, in *QueryMissionControlRequest, opts ...grpc.CallOption) (*QueryMissionControlResponse, error)
	//
	//XImportMissionControl is an experimental API that imports the state provided
	//to the internal mission control's state, using all results which are more
	//recent than our existing values. These values will only be imported
	//in-memory, and will not be persisted across restarts.
	XImportMissionControl(ctx context.Context, in *XImportMissionControlRequest, opts ...grpc.CallOption) (*XImportMissionControlResponse, error)
	//
	//GetMissionControlConfig returns mission control's current config.
	GetMissionControlConfig(ctx context.Context, in *GetMissionControlConfigRequest, opts ...grpc.CallOption) (*GetMissionControlConfigResponse, error)
	//
	//SetMissionControlConfig will set mission control's config, if the config
	//provided is valid.
	SetMissionControlConfig(ctx context.Context, in *SetMissionControlConfigRequest, opts ...grpc.CallOption) (*SetMissionControlConfigResponse, error)
	//
	//QueryProbability returns the current success probability estimate for a
	//given node pair and amount.
	QueryProbability(ctx context.Context, in *QueryProbabilityRequest, opts ...grpc.CallOption) (*QueryProbabilityResponse, error)
	//
	//BuildRoute builds a fully specified route based on a list of hop public
	//keys. It retrieves the relevant channel policies from the graph in order to
	//calculate the correct fees and time locks.
	BuildRoute(ctx context.Context, in *BuildRouteRequest, opts ...grpc.CallOption) (*BuildRouteResponse, error)
	//
	//SubscribeHtlcEvents creates a uni-directional stream from the server to
	//the client which delivers a stream of htlc events.
	SubscribeHtlcEvents(ctx context.Context, in *SubscribeHtlcEventsRequest, opts ...grpc.CallOption) (Router_SubscribeHtlcEventsClient, error)
	// Deprecated: Do not use.
	//
	//Deprecated, use SendPaymentV2. SendPayment attempts to route a payment
	//described by the passed PaymentRequest to the final destination. The call
	//returns a stream of payment status updates.
	SendPayment(ctx context.Context, in *SendPaymentRequest, opts ...grpc.CallOption) (Router_SendPaymentClient, error)
	// Deprecated: Do not use.
	//
	//Deprecated, use TrackPaymentV2. TrackPayment returns an update stream for
	//the payment identified by the payment hash.
	TrackPayment(ctx context.Context, in *TrackPaymentRequest, opts ...grpc.CallOption) (Router_TrackPaymentClient, error)
	//*
	//HtlcInterceptor dispatches a bi-directional streaming RPC in which
	//Forwarded HTLC requests are sent to the client and the client responds with
	//a boolean that tells LND if this htlc should be intercepted.
	//In case of interception, the htlc can be either settled, cancelled or
	//resumed later by using the ResolveHoldForward endpoint.
	HtlcInterceptor(ctx context.Context, opts ...grpc.CallOption) (Router_HtlcInterceptorClient, error)
	//
	//UpdateChanStatus attempts to manually set the state of a channel
	//(enabled, disabled, or auto). A manual "disable" request will cause the
	//channel to stay disabled until a subsequent manual request of either
	//"enable" or "auto".
	UpdateChanStatus(ctx context.Context, in *UpdateChanStatusRequest, opts ...grpc.CallOption) (*UpdateChanStatusResponse, error)
}

type routerClient struct {
	cc grpc.ClientConnInterface
}

func NewRouterClient(cc grpc.ClientConnInterface) RouterClient {
	return &routerClient{cc}
}

func (c *routerClient) OB_SendPaymentV2(ctx context.Context, in *SendPaymentRequest, opts ...grpc.CallOption) (Router_OB_SendPaymentV2Client, error) {
	stream, err := c.cc.NewStream(ctx, &Router_ServiceDesc.Streams[0], "/routerrpc.Router/OB_SendPaymentV2", opts...)
	if err != nil {
		return nil, err
	}
	x := &routerOB_SendPaymentV2Client{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Router_OB_SendPaymentV2Client interface {
	Recv() (*lnrpc.Payment, error)
	grpc.ClientStream
}

type routerOB_SendPaymentV2Client struct {
	grpc.ClientStream
}

func (x *routerOB_SendPaymentV2Client) Recv() (*lnrpc.Payment, error) {
	m := new(lnrpc.Payment)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *routerClient) OB_TrackPaymentV2(ctx context.Context, in *TrackPaymentRequest, opts ...grpc.CallOption) (Router_OB_TrackPaymentV2Client, error) {
	stream, err := c.cc.NewStream(ctx, &Router_ServiceDesc.Streams[1], "/routerrpc.Router/OB_TrackPaymentV2", opts...)
	if err != nil {
		return nil, err
	}
	x := &routerOB_TrackPaymentV2Client{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Router_OB_TrackPaymentV2Client interface {
	Recv() (*lnrpc.Payment, error)
	grpc.ClientStream
}

type routerOB_TrackPaymentV2Client struct {
	grpc.ClientStream
}

func (x *routerOB_TrackPaymentV2Client) Recv() (*lnrpc.Payment, error) {
	m := new(lnrpc.Payment)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *routerClient) EstimateRouteFee(ctx context.Context, in *RouteFeeRequest, opts ...grpc.CallOption) (*RouteFeeResponse, error) {
	out := new(RouteFeeResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/EstimateRouteFee", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Deprecated: Do not use.
func (c *routerClient) SendToRoute(ctx context.Context, in *SendToRouteRequest, opts ...grpc.CallOption) (*SendToRouteResponse, error) {
	out := new(SendToRouteResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/SendToRoute", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) SendToRouteV2(ctx context.Context, in *SendToRouteRequest, opts ...grpc.CallOption) (*lnrpc.HTLCAttempt, error) {
	out := new(lnrpc.HTLCAttempt)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/SendToRouteV2", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) ResetMissionControl(ctx context.Context, in *ResetMissionControlRequest, opts ...grpc.CallOption) (*ResetMissionControlResponse, error) {
	out := new(ResetMissionControlResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/ResetMissionControl", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) QueryMissionControl(ctx context.Context, in *QueryMissionControlRequest, opts ...grpc.CallOption) (*QueryMissionControlResponse, error) {
	out := new(QueryMissionControlResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/QueryMissionControl", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) XImportMissionControl(ctx context.Context, in *XImportMissionControlRequest, opts ...grpc.CallOption) (*XImportMissionControlResponse, error) {
	out := new(XImportMissionControlResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/XImportMissionControl", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) GetMissionControlConfig(ctx context.Context, in *GetMissionControlConfigRequest, opts ...grpc.CallOption) (*GetMissionControlConfigResponse, error) {
	out := new(GetMissionControlConfigResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/GetMissionControlConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) SetMissionControlConfig(ctx context.Context, in *SetMissionControlConfigRequest, opts ...grpc.CallOption) (*SetMissionControlConfigResponse, error) {
	out := new(SetMissionControlConfigResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/SetMissionControlConfig", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) QueryProbability(ctx context.Context, in *QueryProbabilityRequest, opts ...grpc.CallOption) (*QueryProbabilityResponse, error) {
	out := new(QueryProbabilityResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/QueryProbability", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) BuildRoute(ctx context.Context, in *BuildRouteRequest, opts ...grpc.CallOption) (*BuildRouteResponse, error) {
	out := new(BuildRouteResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/BuildRoute", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *routerClient) SubscribeHtlcEvents(ctx context.Context, in *SubscribeHtlcEventsRequest, opts ...grpc.CallOption) (Router_SubscribeHtlcEventsClient, error) {
	stream, err := c.cc.NewStream(ctx, &Router_ServiceDesc.Streams[2], "/routerrpc.Router/SubscribeHtlcEvents", opts...)
	if err != nil {
		return nil, err
	}
	x := &routerSubscribeHtlcEventsClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Router_SubscribeHtlcEventsClient interface {
	Recv() (*HtlcEvent, error)
	grpc.ClientStream
}

type routerSubscribeHtlcEventsClient struct {
	grpc.ClientStream
}

func (x *routerSubscribeHtlcEventsClient) Recv() (*HtlcEvent, error) {
	m := new(HtlcEvent)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Deprecated: Do not use.
func (c *routerClient) SendPayment(ctx context.Context, in *SendPaymentRequest, opts ...grpc.CallOption) (Router_SendPaymentClient, error) {
	stream, err := c.cc.NewStream(ctx, &Router_ServiceDesc.Streams[3], "/routerrpc.Router/SendPayment", opts...)
	if err != nil {
		return nil, err
	}
	x := &routerSendPaymentClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Router_SendPaymentClient interface {
	Recv() (*PaymentStatus, error)
	grpc.ClientStream
}

type routerSendPaymentClient struct {
	grpc.ClientStream
}

func (x *routerSendPaymentClient) Recv() (*PaymentStatus, error) {
	m := new(PaymentStatus)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// Deprecated: Do not use.
func (c *routerClient) TrackPayment(ctx context.Context, in *TrackPaymentRequest, opts ...grpc.CallOption) (Router_TrackPaymentClient, error) {
	stream, err := c.cc.NewStream(ctx, &Router_ServiceDesc.Streams[4], "/routerrpc.Router/TrackPayment", opts...)
	if err != nil {
		return nil, err
	}
	x := &routerTrackPaymentClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type Router_TrackPaymentClient interface {
	Recv() (*PaymentStatus, error)
	grpc.ClientStream
}

type routerTrackPaymentClient struct {
	grpc.ClientStream
}

func (x *routerTrackPaymentClient) Recv() (*PaymentStatus, error) {
	m := new(PaymentStatus)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *routerClient) HtlcInterceptor(ctx context.Context, opts ...grpc.CallOption) (Router_HtlcInterceptorClient, error) {
	stream, err := c.cc.NewStream(ctx, &Router_ServiceDesc.Streams[5], "/routerrpc.Router/HtlcInterceptor", opts...)
	if err != nil {
		return nil, err
	}
	x := &routerHtlcInterceptorClient{stream}
	return x, nil
}

type Router_HtlcInterceptorClient interface {
	Send(*ForwardHtlcInterceptResponse) error
	Recv() (*ForwardHtlcInterceptRequest, error)
	grpc.ClientStream
}

type routerHtlcInterceptorClient struct {
	grpc.ClientStream
}

func (x *routerHtlcInterceptorClient) Send(m *ForwardHtlcInterceptResponse) error {
	return x.ClientStream.SendMsg(m)
}

func (x *routerHtlcInterceptorClient) Recv() (*ForwardHtlcInterceptRequest, error) {
	m := new(ForwardHtlcInterceptRequest)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *routerClient) UpdateChanStatus(ctx context.Context, in *UpdateChanStatusRequest, opts ...grpc.CallOption) (*UpdateChanStatusResponse, error) {
	out := new(UpdateChanStatusResponse)
	err := c.cc.Invoke(ctx, "/routerrpc.Router/UpdateChanStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RouterServer is the server API for Router service.
// All implementations must embed UnimplementedRouterServer
// for forward compatibility
type RouterServer interface {
	//
	//SendPaymentV2 attempts to route a payment described by the passed
	//PaymentRequest to the final destination. The call returns a stream of
	//payment updates.
	OB_SendPaymentV2(*SendPaymentRequest, Router_OB_SendPaymentV2Server) error
	//
	//TrackPaymentV2 returns an update stream for the payment identified by the
	//payment hash.
	OB_TrackPaymentV2(*TrackPaymentRequest, Router_OB_TrackPaymentV2Server) error
	//
	//EstimateRouteFee allows callers to obtain a lower bound w.r.t how much it
	//may cost to send an HTLC to the target end destination.
	EstimateRouteFee(context.Context, *RouteFeeRequest) (*RouteFeeResponse, error)
	// Deprecated: Do not use.
	//
	//Deprecated, use SendToRouteV2. SendToRoute attempts to make a payment via
	//the specified route. This method differs from SendPayment in that it
	//allows users to specify a full route manually. This can be used for
	//things like rebalancing, and atomic swaps. It differs from the newer
	//SendToRouteV2 in that it doesn't return the full HTLC information.
	SendToRoute(context.Context, *SendToRouteRequest) (*SendToRouteResponse, error)
	//
	//SendToRouteV2 attempts to make a payment via the specified route. This
	//method differs from SendPayment in that it allows users to specify a full
	//route manually. This can be used for things like rebalancing, and atomic
	//swaps.
	SendToRouteV2(context.Context, *SendToRouteRequest) (*lnrpc.HTLCAttempt, error)
	//
	//ResetMissionControl clears all mission control state and starts with a clean
	//slate.
	ResetMissionControl(context.Context, *ResetMissionControlRequest) (*ResetMissionControlResponse, error)
	//
	//QueryMissionControl exposes the internal mission control state to callers.
	//It is a development feature.
	QueryMissionControl(context.Context, *QueryMissionControlRequest) (*QueryMissionControlResponse, error)
	//
	//XImportMissionControl is an experimental API that imports the state provided
	//to the internal mission control's state, using all results which are more
	//recent than our existing values. These values will only be imported
	//in-memory, and will not be persisted across restarts.
	XImportMissionControl(context.Context, *XImportMissionControlRequest) (*XImportMissionControlResponse, error)
	//
	//GetMissionControlConfig returns mission control's current config.
	GetMissionControlConfig(context.Context, *GetMissionControlConfigRequest) (*GetMissionControlConfigResponse, error)
	//
	//SetMissionControlConfig will set mission control's config, if the config
	//provided is valid.
	SetMissionControlConfig(context.Context, *SetMissionControlConfigRequest) (*SetMissionControlConfigResponse, error)
	//
	//QueryProbability returns the current success probability estimate for a
	//given node pair and amount.
	QueryProbability(context.Context, *QueryProbabilityRequest) (*QueryProbabilityResponse, error)
	//
	//BuildRoute builds a fully specified route based on a list of hop public
	//keys. It retrieves the relevant channel policies from the graph in order to
	//calculate the correct fees and time locks.
	BuildRoute(context.Context, *BuildRouteRequest) (*BuildRouteResponse, error)
	//
	//SubscribeHtlcEvents creates a uni-directional stream from the server to
	//the client which delivers a stream of htlc events.
	SubscribeHtlcEvents(*SubscribeHtlcEventsRequest, Router_SubscribeHtlcEventsServer) error
	// Deprecated: Do not use.
	//
	//Deprecated, use SendPaymentV2. SendPayment attempts to route a payment
	//described by the passed PaymentRequest to the final destination. The call
	//returns a stream of payment status updates.
	SendPayment(*SendPaymentRequest, Router_SendPaymentServer) error
	// Deprecated: Do not use.
	//
	//Deprecated, use TrackPaymentV2. TrackPayment returns an update stream for
	//the payment identified by the payment hash.
	TrackPayment(*TrackPaymentRequest, Router_TrackPaymentServer) error
	//*
	//HtlcInterceptor dispatches a bi-directional streaming RPC in which
	//Forwarded HTLC requests are sent to the client and the client responds with
	//a boolean that tells LND if this htlc should be intercepted.
	//In case of interception, the htlc can be either settled, cancelled or
	//resumed later by using the ResolveHoldForward endpoint.
	HtlcInterceptor(Router_HtlcInterceptorServer) error
	//
	//UpdateChanStatus attempts to manually set the state of a channel
	//(enabled, disabled, or auto). A manual "disable" request will cause the
	//channel to stay disabled until a subsequent manual request of either
	//"enable" or "auto".
	UpdateChanStatus(context.Context, *UpdateChanStatusRequest) (*UpdateChanStatusResponse, error)
	mustEmbedUnimplementedRouterServer()
}

// UnimplementedRouterServer must be embedded to have forward compatible implementations.
type UnimplementedRouterServer struct {
}

func (UnimplementedRouterServer) OB_SendPaymentV2(*SendPaymentRequest, Router_OB_SendPaymentV2Server) error {
	return status.Errorf(codes.Unimplemented, "method OB_SendPaymentV2 not implemented")
}
func (UnimplementedRouterServer) OB_TrackPaymentV2(*TrackPaymentRequest, Router_OB_TrackPaymentV2Server) error {
	return status.Errorf(codes.Unimplemented, "method OB_TrackPaymentV2 not implemented")
}
func (UnimplementedRouterServer) EstimateRouteFee(context.Context, *RouteFeeRequest) (*RouteFeeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method EstimateRouteFee not implemented")
}
func (UnimplementedRouterServer) SendToRoute(context.Context, *SendToRouteRequest) (*SendToRouteResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendToRoute not implemented")
}
func (UnimplementedRouterServer) SendToRouteV2(context.Context, *SendToRouteRequest) (*lnrpc.HTLCAttempt, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SendToRouteV2 not implemented")
}
func (UnimplementedRouterServer) ResetMissionControl(context.Context, *ResetMissionControlRequest) (*ResetMissionControlResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ResetMissionControl not implemented")
}
func (UnimplementedRouterServer) QueryMissionControl(context.Context, *QueryMissionControlRequest) (*QueryMissionControlResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryMissionControl not implemented")
}
func (UnimplementedRouterServer) XImportMissionControl(context.Context, *XImportMissionControlRequest) (*XImportMissionControlResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method XImportMissionControl not implemented")
}
func (UnimplementedRouterServer) GetMissionControlConfig(context.Context, *GetMissionControlConfigRequest) (*GetMissionControlConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetMissionControlConfig not implemented")
}
func (UnimplementedRouterServer) SetMissionControlConfig(context.Context, *SetMissionControlConfigRequest) (*SetMissionControlConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method SetMissionControlConfig not implemented")
}
func (UnimplementedRouterServer) QueryProbability(context.Context, *QueryProbabilityRequest) (*QueryProbabilityResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method QueryProbability not implemented")
}
func (UnimplementedRouterServer) BuildRoute(context.Context, *BuildRouteRequest) (*BuildRouteResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method BuildRoute not implemented")
}
func (UnimplementedRouterServer) SubscribeHtlcEvents(*SubscribeHtlcEventsRequest, Router_SubscribeHtlcEventsServer) error {
	return status.Errorf(codes.Unimplemented, "method SubscribeHtlcEvents not implemented")
}
func (UnimplementedRouterServer) SendPayment(*SendPaymentRequest, Router_SendPaymentServer) error {
	return status.Errorf(codes.Unimplemented, "method SendPayment not implemented")
}
func (UnimplementedRouterServer) TrackPayment(*TrackPaymentRequest, Router_TrackPaymentServer) error {
	return status.Errorf(codes.Unimplemented, "method TrackPayment not implemented")
}
func (UnimplementedRouterServer) HtlcInterceptor(Router_HtlcInterceptorServer) error {
	return status.Errorf(codes.Unimplemented, "method HtlcInterceptor not implemented")
}
func (UnimplementedRouterServer) UpdateChanStatus(context.Context, *UpdateChanStatusRequest) (*UpdateChanStatusResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateChanStatus not implemented")
}
func (UnimplementedRouterServer) mustEmbedUnimplementedRouterServer() {}

// UnsafeRouterServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RouterServer will
// result in compilation errors.
type UnsafeRouterServer interface {
	mustEmbedUnimplementedRouterServer()
}

func RegisterRouterServer(s grpc.ServiceRegistrar, srv RouterServer) {
	s.RegisterService(&Router_ServiceDesc, srv)
}

func _Router_OB_SendPaymentV2_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SendPaymentRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(RouterServer).OB_SendPaymentV2(m, &routerOB_SendPaymentV2Server{stream})
}

type Router_OB_SendPaymentV2Server interface {
	Send(*lnrpc.Payment) error
	grpc.ServerStream
}

type routerOB_SendPaymentV2Server struct {
	grpc.ServerStream
}

func (x *routerOB_SendPaymentV2Server) Send(m *lnrpc.Payment) error {
	return x.ServerStream.SendMsg(m)
}

func _Router_OB_TrackPaymentV2_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(TrackPaymentRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(RouterServer).OB_TrackPaymentV2(m, &routerOB_TrackPaymentV2Server{stream})
}

type Router_OB_TrackPaymentV2Server interface {
	Send(*lnrpc.Payment) error
	grpc.ServerStream
}

type routerOB_TrackPaymentV2Server struct {
	grpc.ServerStream
}

func (x *routerOB_TrackPaymentV2Server) Send(m *lnrpc.Payment) error {
	return x.ServerStream.SendMsg(m)
}

func _Router_EstimateRouteFee_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(RouteFeeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).EstimateRouteFee(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/EstimateRouteFee",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).EstimateRouteFee(ctx, req.(*RouteFeeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_SendToRoute_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendToRouteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).SendToRoute(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/SendToRoute",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).SendToRoute(ctx, req.(*SendToRouteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_SendToRouteV2_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendToRouteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).SendToRouteV2(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/SendToRouteV2",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).SendToRouteV2(ctx, req.(*SendToRouteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_ResetMissionControl_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResetMissionControlRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).ResetMissionControl(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/ResetMissionControl",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).ResetMissionControl(ctx, req.(*ResetMissionControlRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_QueryMissionControl_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryMissionControlRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).QueryMissionControl(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/QueryMissionControl",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).QueryMissionControl(ctx, req.(*QueryMissionControlRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_XImportMissionControl_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(XImportMissionControlRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).XImportMissionControl(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/XImportMissionControl",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).XImportMissionControl(ctx, req.(*XImportMissionControlRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_GetMissionControlConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMissionControlConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).GetMissionControlConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/GetMissionControlConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).GetMissionControlConfig(ctx, req.(*GetMissionControlConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_SetMissionControlConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SetMissionControlConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).SetMissionControlConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/SetMissionControlConfig",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).SetMissionControlConfig(ctx, req.(*SetMissionControlConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_QueryProbability_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(QueryProbabilityRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).QueryProbability(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/QueryProbability",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).QueryProbability(ctx, req.(*QueryProbabilityRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_BuildRoute_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BuildRouteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).BuildRoute(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/BuildRoute",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).BuildRoute(ctx, req.(*BuildRouteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Router_SubscribeHtlcEvents_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SubscribeHtlcEventsRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(RouterServer).SubscribeHtlcEvents(m, &routerSubscribeHtlcEventsServer{stream})
}

type Router_SubscribeHtlcEventsServer interface {
	Send(*HtlcEvent) error
	grpc.ServerStream
}

type routerSubscribeHtlcEventsServer struct {
	grpc.ServerStream
}

func (x *routerSubscribeHtlcEventsServer) Send(m *HtlcEvent) error {
	return x.ServerStream.SendMsg(m)
}

func _Router_SendPayment_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(SendPaymentRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(RouterServer).SendPayment(m, &routerSendPaymentServer{stream})
}

type Router_SendPaymentServer interface {
	Send(*PaymentStatus) error
	grpc.ServerStream
}

type routerSendPaymentServer struct {
	grpc.ServerStream
}

func (x *routerSendPaymentServer) Send(m *PaymentStatus) error {
	return x.ServerStream.SendMsg(m)
}

func _Router_TrackPayment_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(TrackPaymentRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(RouterServer).TrackPayment(m, &routerTrackPaymentServer{stream})
}

type Router_TrackPaymentServer interface {
	Send(*PaymentStatus) error
	grpc.ServerStream
}

type routerTrackPaymentServer struct {
	grpc.ServerStream
}

func (x *routerTrackPaymentServer) Send(m *PaymentStatus) error {
	return x.ServerStream.SendMsg(m)
}

func _Router_HtlcInterceptor_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(RouterServer).HtlcInterceptor(&routerHtlcInterceptorServer{stream})
}

type Router_HtlcInterceptorServer interface {
	Send(*ForwardHtlcInterceptRequest) error
	Recv() (*ForwardHtlcInterceptResponse, error)
	grpc.ServerStream
}

type routerHtlcInterceptorServer struct {
	grpc.ServerStream
}

func (x *routerHtlcInterceptorServer) Send(m *ForwardHtlcInterceptRequest) error {
	return x.ServerStream.SendMsg(m)
}

func (x *routerHtlcInterceptorServer) Recv() (*ForwardHtlcInterceptResponse, error) {
	m := new(ForwardHtlcInterceptResponse)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _Router_UpdateChanStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateChanStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RouterServer).UpdateChanStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/routerrpc.Router/UpdateChanStatus",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RouterServer).UpdateChanStatus(ctx, req.(*UpdateChanStatusRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Router_ServiceDesc is the grpc.ServiceDesc for Router service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Router_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "routerrpc.Router",
	HandlerType: (*RouterServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "EstimateRouteFee",
			Handler:    _Router_EstimateRouteFee_Handler,
		},
		{
			MethodName: "SendToRoute",
			Handler:    _Router_SendToRoute_Handler,
		},
		{
			MethodName: "SendToRouteV2",
			Handler:    _Router_SendToRouteV2_Handler,
		},
		{
			MethodName: "ResetMissionControl",
			Handler:    _Router_ResetMissionControl_Handler,
		},
		{
			MethodName: "QueryMissionControl",
			Handler:    _Router_QueryMissionControl_Handler,
		},
		{
			MethodName: "XImportMissionControl",
			Handler:    _Router_XImportMissionControl_Handler,
		},
		{
			MethodName: "GetMissionControlConfig",
			Handler:    _Router_GetMissionControlConfig_Handler,
		},
		{
			MethodName: "SetMissionControlConfig",
			Handler:    _Router_SetMissionControlConfig_Handler,
		},
		{
			MethodName: "QueryProbability",
			Handler:    _Router_QueryProbability_Handler,
		},
		{
			MethodName: "BuildRoute",
			Handler:    _Router_BuildRoute_Handler,
		},
		{
			MethodName: "UpdateChanStatus",
			Handler:    _Router_UpdateChanStatus_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "OB_SendPaymentV2",
			Handler:       _Router_OB_SendPaymentV2_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "OB_TrackPaymentV2",
			Handler:       _Router_OB_TrackPaymentV2_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "SubscribeHtlcEvents",
			Handler:       _Router_SubscribeHtlcEvents_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "SendPayment",
			Handler:       _Router_SendPayment_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "TrackPayment",
			Handler:       _Router_TrackPayment_Handler,
			ServerStreams: true,
		},
		{
			StreamName:    "HtlcInterceptor",
			Handler:       _Router_HtlcInterceptor_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "routerrpc/router.proto",
}
