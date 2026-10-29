// Package reqctx membawa request_id melalui context agar audit dan log dapat dikorelasikan.
package reqctx

import "context"

type key struct{}

func With(ctx context.Context, id string) context.Context { return context.WithValue(ctx, key{}, id) }

func ID(ctx context.Context) string {
	v, _ := ctx.Value(key{}).(string)
	return v
}
