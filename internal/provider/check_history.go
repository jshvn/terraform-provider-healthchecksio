package provider

import (
	"context"
	"encoding/base64"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// checkIDAttribute is the check_id input of the history data sources and the actions.
func checkIDAttribute() schema.StringAttribute {
	return schema.StringAttribute{
		MarkdownDescription: "The check: its unique_key (the `id` of `healthchecksio_check`) or its UUID.",
		Required:            true,
	}
}

// pingsDataSource gives the logged pings of one check.
type pingsDataSource struct{ dataSourceBase }

func newPingsDataSource() datasource.DataSource {
	return &pingsDataSource{dataSourceBase{name: "pings"}}
}

type pingModel struct {
	Type       types.String  `tfsdk:"type"`
	Date       types.String  `tfsdk:"date"`
	N          types.Int64   `tfsdk:"n"`
	Scheme     types.String  `tfsdk:"scheme"`
	RemoteAddr types.String  `tfsdk:"remote_addr"`
	Method     types.String  `tfsdk:"method"`
	UA         types.String  `tfsdk:"ua"`
	RID        types.String  `tfsdk:"rid"`
	Duration   types.Float64 `tfsdk:"duration"`
	HasBody    types.Bool    `tfsdk:"has_body"`
}

type pingsModel struct {
	CheckID types.String `tfsdk:"check_id"`
	Pings   []pingModel  `tfsdk:"pings"`
}

func (d *pingsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	str := func(desc string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "The logged pings of one check, the newest first. A free account keeps 100 pings, and a paid account keeps 1000. This data source needs a read-write API key.",
		Attributes: map[string]schema.Attribute{
			"check_id": checkIDAttribute(),
			"pings": schema.ListNestedAttribute{
				MarkdownDescription: "The pings.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"type":        str("The kind of the ping, for example `success`, `start`, `fail` or `log`."),
					"date":        str("The time of the ping (RFC 3339)."),
					"n":           schema.Int64Attribute{MarkdownDescription: "The number of the ping. Use it with `healthchecksio_ping_body`.", Computed: true},
					"scheme":      str("The scheme of the ping: `http`, `https` or `email`."),
					"remote_addr": str("The address of the sender."),
					"method":      str("The HTTP method of the ping."),
					"ua":          str("The user agent of the sender."),
					"rid":         str("The run ID of the ping, or null."),
					"duration":    schema.Float64Attribute{MarkdownDescription: "The time from the start signal to this signal, in seconds, or null.", Computed: true},
					"has_body":    schema.BoolAttribute{MarkdownDescription: "True when the server keeps a body for this ping.", Computed: true},
				}},
			},
		},
	}
}

func (d *pingsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg pingsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	uuid, err := resolveUUID(ctx, d.c, cfg.CheckID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Cannot find the check", err.Error())
		return
	}
	pings, err := d.c.ListPings(ctx, uuid)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list the pings", err.Error())
		return
	}
	cfg.Pings = make([]pingModel, 0, len(pings))
	for _, p := range pings {
		cfg.Pings = append(cfg.Pings, pingModel{
			Type: types.StringValue(p.Type), Date: types.StringValue(p.Date), N: types.Int64Value(p.N),
			Scheme: types.StringValue(p.Scheme), RemoteAddr: types.StringValue(p.RemoteAddr),
			Method: types.StringValue(p.Method), UA: types.StringValue(p.UA), RID: types.StringPointerValue(p.RID),
			Duration: types.Float64PointerValue(p.Duration), HasBody: types.BoolValue(p.BodyURL != nil),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// pingBodyDataSource gives the logged body of one ping. The body URL contains the UUID
// of the check. Thus, pings has has_body in its place, and this data source gets the
// body.
type pingBodyDataSource struct{ dataSourceBase }

func newPingBodyDataSource() datasource.DataSource {
	return &pingBodyDataSource{dataSourceBase{name: "ping_body"}}
}

type pingBodyModel struct {
	CheckID    types.String `tfsdk:"check_id"`
	N          types.Int64  `tfsdk:"n"`
	Body       types.String `tfsdk:"body"`
	BodyBase64 types.String `tfsdk:"body_base64"`
}

func (d *pingBodyDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The logged request body of one ping. This data source needs a read-write API key.",
		Attributes: map[string]schema.Attribute{
			"check_id":    checkIDAttribute(),
			"n":           schema.Int64Attribute{MarkdownDescription: "The number of the ping, from `healthchecksio_pings`.", Required: true, Validators: []validator.Int64{int64validator.AtLeast(1)}},
			"body":        schema.StringAttribute{MarkdownDescription: "The body, as text. Null when the body is not valid UTF-8. A body can contain secrets. Thus, it is sensitive.", Computed: true, Sensitive: true},
			"body_base64": schema.StringAttribute{MarkdownDescription: "The body, base64-encoded. Use it for a binary body.", Computed: true, Sensitive: true},
		},
	}
}

func (d *pingBodyDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg pingBodyModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	uuid, err := resolveUUID(ctx, d.c, cfg.CheckID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Cannot find the check", err.Error())
		return
	}
	body, err := d.c.PingBody(ctx, uuid, cfg.N.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Cannot read the ping body", err.Error())
		return
	}
	cfg.Body = types.StringNull()
	if utf8.Valid(body) {
		cfg.Body = types.StringValue(string(body))
	}
	cfg.BodyBase64 = types.StringValue(base64.StdEncoding.EncodeToString(body))
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// flipsDataSource gives the status changes of one check.
type flipsDataSource struct{ dataSourceBase }

func newFlipsDataSource() datasource.DataSource {
	return &flipsDataSource{dataSourceBase{name: "flips"}}
}

type flipModel struct {
	Timestamp types.String `tfsdk:"timestamp"`
	Up        types.Bool   `tfsdk:"up"`
}

type flipsModel struct {
	CheckID types.String `tfsdk:"check_id"`
	Seconds types.Int64  `tfsdk:"seconds"`
	Start   types.Int64  `tfsdk:"start"`
	End     types.Int64  `tfsdk:"end"`
	Flips   []flipModel  `tfsdk:"flips"`
}

func (d *flipsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	when := func(desc string) schema.Attribute {
		return schema.Int64Attribute{MarkdownDescription: desc, Optional: true, Validators: []validator.Int64{int64validator.AtLeast(1)}}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "The status changes (flips) of one check. The server keeps the flips of this month and of the two months before it. A read-only API key is sufficient.",
		Attributes: map[string]schema.Attribute{
			"check_id": checkIDAttribute(),
			"seconds":  when("Give only the flips of the last number of seconds."),
			"start":    when("Give only the flips at or after this UNIX time."),
			"end":      when("Give only the flips before this UNIX time."),
			"flips": schema.ListNestedAttribute{
				MarkdownDescription: "The flips, the newest first.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"timestamp": schema.StringAttribute{MarkdownDescription: "The time of the flip (RFC 3339).", Computed: true},
					"up":        schema.BoolAttribute{MarkdownDescription: "True for a change to up, false for a change to down.", Computed: true},
				}},
			},
		},
	}
}

func (d *flipsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg flipsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	filter := client.FlipFilter{Seconds: cfg.Seconds.ValueInt64(), Start: cfg.Start.ValueInt64(), End: cfg.End.ValueInt64()}
	flips, err := d.c.ListFlips(ctx, cfg.CheckID.ValueString(), filter)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list the flips", err.Error())
		return
	}
	cfg.Flips = make([]flipModel, 0, len(flips))
	for _, f := range flips {
		cfg.Flips = append(cfg.Flips, flipModel{types.StringValue(f.Timestamp), types.BoolValue(f.Up == 1)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
