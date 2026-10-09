package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// The API defaults (hc/api/models.py). The schema uses them, and an unset timeout gets
// defaultTimeout.
const (
	defaultTimeout = 86400
	defaultGrace   = 3600
	defaultTZ      = "UTC"
	minPeriod      = 60
	maxPeriod      = 31536000
)

// uuidPattern matches a UUID in uppercase or in lowercase.
const uuidPattern = `[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`

var uuidRE = regexp.MustCompile("^" + uuidPattern + "$")

// lowerUUIDRE matches a UUID as the server writes it. An attribute that the server
// returns must use this form. If it does not, the state does not agree with the plan
// after the apply.
var lowerUUIDRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// uniqueKeyRE matches a unique_key: 40 lowercase hex digits.
var uniqueKeyRE = regexp.MustCompile(`^[0-9a-f]{40}$`)

// checkFields contains the attributes that the resource, the list resource and the
// check data sources share. Each name is the name of the API field, but tags, channels
// and the keywords are sets, not strings.
type checkFields struct {
	ID                types.String `tfsdk:"id"`
	UUID              types.String `tfsdk:"uuid"`
	PingURL           types.String `tfsdk:"ping_url"`
	BadgeURL          types.String `tfsdk:"badge_url"`
	Name              types.String `tfsdk:"name"`
	Slug              types.String `tfsdk:"slug"`
	Tags              types.Set    `tfsdk:"tags"`
	Desc              types.String `tfsdk:"desc"`
	Timeout           types.Int64  `tfsdk:"timeout"`
	Schedule          types.String `tfsdk:"schedule"`
	TZ                types.String `tfsdk:"tz"`
	Grace             types.Int64  `tfsdk:"grace"`
	ManualResume      types.Bool   `tfsdk:"manual_resume"`
	Methods           types.String `tfsdk:"methods"`
	Channels          types.Set    `tfsdk:"channels"`
	StartKw           types.Set    `tfsdk:"start_kw"`
	SuccessKw         types.Set    `tfsdk:"success_kw"`
	FailureKw         types.Set    `tfsdk:"failure_kw"`
	FilterSubject     types.Bool   `tfsdk:"filter_subject"`
	FilterBody        types.Bool   `tfsdk:"filter_body"`
	FilterHTTPBody    types.Bool   `tfsdk:"filter_http_body"`
	FilterDefaultFail types.Bool   `tfsdk:"filter_default_fail"`
}

// fieldsFromAPI gives the attributes for one API check. The id is the unique_key. With a
// read-only key, the API does not send uuid, ping_url and channels. Thus, they are null.
func fieldsFromAPI(c *client.Check) checkFields {
	f := checkFields{
		ID:                types.StringValue(c.Key()),
		UUID:              optString(c.UUID),
		PingURL:           optString(c.PingURL),
		BadgeURL:          types.StringValue(c.BadgeURL),
		Name:              types.StringValue(c.Name),
		Slug:              types.StringValue(c.Slug),
		Tags:              stringSet(strings.Fields(c.Tags)),
		Desc:              types.StringValue(c.Desc),
		Timeout:           types.Int64Null(),
		Schedule:          types.StringNull(),
		TZ:                types.StringNull(),
		Grace:             types.Int64Value(c.Grace),
		ManualResume:      types.BoolValue(c.ManualResume),
		Methods:           types.StringValue(c.Methods),
		Channels:          types.SetNull(types.StringType),
		StartKw:           stringSet(splitList(c.StartKw)),
		SuccessKw:         stringSet(splitList(c.SuccessKw)),
		FailureKw:         stringSet(splitList(c.FailureKw)),
		FilterSubject:     types.BoolValue(c.FilterSubject),
		FilterBody:        types.BoolValue(c.FilterBody),
		FilterHTTPBody:    types.BoolValue(c.FilterHTTPBody),
		FilterDefaultFail: types.BoolValue(c.FilterDefaultFail),
	}
	if c.Schedule != nil {
		f.Schedule = types.StringValue(*c.Schedule)
		f.TZ = types.StringPointerValue(c.TZ)
	} else {
		f.Timeout = types.Int64PointerValue(c.Timeout)
	}
	if c.UUID != "" {
		f.Channels = stringSet(splitList(c.Channels))
	}
	return f
}

// spec gives the API body for these attributes. It sends each field, also an empty
// value. Thus, the removal of a value from the configuration clears it on the server.
// It sends channels only when they are known and not null: a null value leaves the
// integrations of the check as they are.
func (f checkFields) spec(ctx context.Context) (client.CheckSpec, diag.Diagnostics) {
	var diags diag.Diagnostics
	join := func(s types.Set, sep string) *string {
		var v []string
		diags.Append(s.ElementsAs(ctx, &v, false)...)
		slices.Sort(v)
		return ptr(strings.Join(v, sep))
	}
	s := client.CheckSpec{
		Name:              ptr(f.Name.ValueString()),
		Slug:              ptr(f.Slug.ValueString()),
		Tags:              join(f.Tags, " "),
		Desc:              ptr(f.Desc.ValueString()),
		Grace:             ptr(f.Grace.ValueInt64()),
		ManualResume:      ptr(f.ManualResume.ValueBool()),
		Methods:           ptr(f.Methods.ValueString()),
		StartKw:           join(f.StartKw, ","),
		SuccessKw:         join(f.SuccessKw, ","),
		FailureKw:         join(f.FailureKw, ","),
		FilterSubject:     ptr(f.FilterSubject.ValueBool()),
		FilterBody:        ptr(f.FilterBody.ValueBool()),
		FilterHTTPBody:    ptr(f.FilterHTTPBody.ValueBool()),
		FilterDefaultFail: ptr(f.FilterDefaultFail.ValueBool()),
	}
	if f.Schedule.IsNull() {
		s.Timeout = ptr(f.Timeout.ValueInt64())
	} else {
		s.Schedule = ptr(f.Schedule.ValueString())
		s.TZ = ptr(f.TZ.ValueString())
	}
	if !f.Channels.IsNull() && !f.Channels.IsUnknown() {
		s.Channels = join(f.Channels, ",")
	}
	return s, diags
}

// resolveUUID gives the UUID of the check with the identifier id. id is a UUID or a
// unique_key. For a unique_key, resolveUUID lists the checks, because the API finds a
// check by unique_key only for reads. This needs a read-write key.
func resolveUUID(ctx context.Context, c *client.Client, id string) (string, error) {
	if uuidRE.MatchString(id) {
		return strings.ToLower(id), nil
	}
	if !client.IsUniqueKey(id) {
		return "", fmt.Errorf("%q is not a check UUID or a unique_key", id)
	}
	checks, err := c.ListChecks(ctx, client.CheckFilter{})
	if err != nil {
		return "", err
	}
	for _, ch := range checks {
		if ch.Key() == id {
			if ch.UUID == "" {
				return "", fmt.Errorf("check %s: the API key is read-only, and this operation needs a read-write key", id)
			}
			return ch.UUID, nil
		}
	}
	return "", fmt.Errorf("no check in this project has the unique_key %s", id)
}

// splitList splits a comma-separated API value. An empty value gives no items.
func splitList(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// stringSet gives a set of the items. A set cannot contain an item two times. Thus,
// stringSet removes duplicates, for example from "a,a" on the server.
func stringSet(items []string) types.Set {
	items = slices.Compact(slices.Sorted(slices.Values(items)))
	values := make([]attr.Value, 0, len(items))
	for _, s := range items {
		values = append(values, types.StringValue(s))
	}
	return types.SetValueMust(types.StringType, values)
}

func optString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func ptr[T any](v T) *T { return &v }
