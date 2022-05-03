package provider_fm

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	hex2 "encoding/hex"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"math/big"
	"strings"
)

type resourceIDType struct{}

func (r resourceIDType) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		Description: `
The resource ` + "`random_id`" + ` generates random numbers that are intended to be
used as unique identifiers for other resources.

This resource *does* use a cryptographic random number generator in order
to minimize the chance of collisions, making the results of this resource
when a 16-byte identifier is requested of equivalent uniqueness to a
type-4 UUID.

This resource can be used in conjunction with resources that have
the ` + "`create_before_destroy`" + ` lifecycle flag set to avoid conflicts with
unique names during the brief period where both the old and new resources
exist concurrently.
`,
		Attributes: map[string]tfsdk.Attribute{
			"keepers": {
				Description: "Arbitrary map of values that, when changed, will trigger recreation of " +
					"resource. See [the main provider documentation](../index.html) for more information.",
				Type: types.MapType{
					ElemType: types.StringType,
				},
				Optional:      true,
				PlanModifiers: []tfsdk.AttributePlanModifier{tfsdk.RequiresReplace()},
			},
			"byte_length": {
				Description: "The number of random bytes to produce. The minimum value is 1, which produces " +
					"eight bits of randomness.",
				Type:          types.Int64Type,
				Required:      true,
				PlanModifiers: []tfsdk.AttributePlanModifier{tfsdk.RequiresReplace()},
			},
			"prefix": {
				Description: "Arbitrary string to prefix the output value with. This string is supplied as-is, " +
					"meaning it is not guaranteed to be URL-safe or base64 encoded.",
				Type:          types.StringType,
				Optional:      true,
				PlanModifiers: []tfsdk.AttributePlanModifier{tfsdk.RequiresReplace()},
			},
			"b64_url": {
				Description: "The generated id presented in base64, using the URL-friendly character set: " +
					"case-sensitive letters, digits and the characters `_` and `-`.",
				Type:     types.StringType,
				Computed: true,
			},
			"b64_std": {
				Description: "The generated id presented in base64 without additional transformations.",
				Type:        types.StringType,
				Computed:    true,
			},
			"hex": {
				Description: "The generated id presented in padded hexadecimal digits. This result will " +
					"always be twice as long as the requested byte length.",
				Type:     types.StringType,
				Computed: true,
			},
			"dec": {
				Description: "The generated id presented in non-padded decimal digits.",
				Type:        types.StringType,
				Computed:    true,
			},
			"id": {
				Description: "The generated id presented in base64 without additional transformations or prefix.",
				Type:        types.StringType,
				Computed:    true,
			},
		},
	}, nil
}

func (r resourceIDType) NewResource(_ context.Context, p tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	return resourceID{
		p: *(p.(*provider)),
	}, nil
}

type resourceID struct {
	p provider
}

func (r resourceID) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	if !r.p.configured {
		resp.Diagnostics.AddError(
			"provider not configured",
			"provider not configured",
		)
	}

	var plan ID

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	byteLength := plan.ByteLength.Value
	bytes := make([]byte, byteLength)

	n, err := rand.Reader.Read(bytes)
	if int64(n) != byteLength {
		resp.Diagnostics.AddError(
			"generated insufficient random bytes: %s",
			fmt.Sprintf("generated insufficient random bytes: %s", err),
		)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(
			"error generating random bytes",
			fmt.Sprintf("error generating random bytes: %s", err),
		)
		return
	}

	id := base64.RawURLEncoding.EncodeToString(bytes)
	prefix := plan.Prefix.Value
	b64Std := base64.StdEncoding.EncodeToString(bytes)
	hex := hex2.EncodeToString(bytes)

	bigInt := big.Int{}
	bigInt.SetBytes(bytes)
	dec := bigInt.String()

	i := ID{
		ID:         types.String{Value: id},
		Keepers:    plan.Keepers,
		ByteLength: types.Int64{Value: plan.ByteLength.Value},
		Prefix:     plan.Prefix,
		B64URL:     types.String{Value: prefix + id},
		B64Std:     types.String{Value: prefix + b64Std},
		Hex:        types.String{Value: prefix + hex},
		Dec:        types.String{Value: prefix + dec},
	}

	diags = resp.State.Set(ctx, i)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r resourceID) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	// Intentionally left blank.
}

func (r resourceID) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	// Intentionally left blank.
}

func (r resourceID) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	resp.State.RemoveResource(ctx)
}

func (r resourceID) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	id := req.ID
	var prefix string

	sep := strings.LastIndex(id, ",")
	if sep != -1 {
		prefix = id[:sep]
		id = id[sep+1:]
	}

	bytes, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		resp.Diagnostics.AddError(
			"error decoding ID",
			fmt.Sprintf("error decoding ID: %s", err))
		return
	}

	b64Std := base64.StdEncoding.EncodeToString(bytes)
	hex := hex2.EncodeToString(bytes)

	bigInt := big.Int{}
	bigInt.SetBytes(bytes)
	dec := bigInt.String()

	var state ID

	state.ID.Value = id
	state.ByteLength.Value = int64(len(bytes))
	state.Keepers.ElemType = types.StringType
	state.B64Std.Value = prefix + b64Std
	state.B64URL.Value = prefix + id
	state.Hex.Value = prefix + hex
	state.Dec.Value = prefix + dec

	if prefix == "" {
		state.Prefix.Null = true
	} else {
		state.Prefix.Value = prefix
	}

	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}