package platform

import "context"

type actorContextKey struct{}
type deviceContextKey struct{}
type requestMetadataContextKey struct{}

type RequestMetadata struct {
	IP        string
	RequestID string
}

func WithRequestMetadata(ctx context.Context, metadata RequestMetadata) context.Context {
	metadata.IP = normalizeAuditIP(metadata.IP)
	return context.WithValue(ctx, requestMetadataContextKey{}, metadata)
}

func RequestMetadataFromContext(ctx context.Context) (RequestMetadata, bool) {
	metadata, ok := ctx.Value(requestMetadataContextKey{}).(RequestMetadata)
	return metadata, ok
}

type DeviceIdentity struct {
	DeviceID   int64
	MerchantID int64
	PublicID   string
	TokenHash  []byte
}

func WithActor(ctx context.Context, actor Actor) context.Context {
	return context.WithValue(ctx, actorContextKey{}, actor)
}
func ActorFromContext(ctx context.Context) (Actor, bool) {
	actor, ok := ctx.Value(actorContextKey{}).(Actor)
	return actor, ok
}
func WithDevice(ctx context.Context, device DeviceIdentity) context.Context {
	return context.WithValue(ctx, deviceContextKey{}, device)
}
func DeviceFromContext(ctx context.Context) (DeviceIdentity, bool) {
	device, ok := ctx.Value(deviceContextKey{}).(DeviceIdentity)
	return device, ok
}
