package provider

import (
	"context"
	"errors"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// dataSourceBase gives Metadata and Configure to a data source. name is the type name
// after the "healthchecksio_" prefix.
type dataSourceBase struct {
	name string
	c    *client.Client
}

func (b *dataSourceBase) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + b.name
}

func (b *dataSourceBase) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	b.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

// channelsDataSource lists the integrations of the project.
type channelsDataSource struct{ dataSourceBase }

func newChannelsDataSource() datasource.DataSource {
	return &channelsDataSource{dataSourceBase{name: "channels"}}
}

type channelModel struct {
	ID   types.String `tfsdk:"id"`
	Name types.String `tfsdk:"name"`
	Kind types.String `tfsdk:"kind"`
}

type channelsModel struct {
	Kind     types.String   `tfsdk:"kind"`
	Name     types.String   `tfsdk:"name"`
	Channels []channelModel `tfsdk:"channels"`
	IDs      types.Set      `tfsdk:"ids"`
}

func (d *channelsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The integrations of the project. The API cannot make or change an integration. Use the dashboard for that. A read-only API key cannot list integrations.",
		Attributes: map[string]schema.Attribute{
			"kind": schema.StringAttribute{MarkdownDescription: "Give only the integrations of this kind, for example `email` or `slack`.", Optional: true},
			"name": schema.StringAttribute{MarkdownDescription: "Give only the integrations with this name.", Optional: true},
			"channels": schema.ListNestedAttribute{
				MarkdownDescription: "The integrations, in the order of the API.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":   schema.StringAttribute{MarkdownDescription: "The UUID of the integration.", Computed: true},
					"name": schema.StringAttribute{MarkdownDescription: "The name of the integration.", Computed: true},
					"kind": schema.StringAttribute{MarkdownDescription: "The kind of the integration.", Computed: true},
				}},
			},
			"ids": schema.SetAttribute{MarkdownDescription: "The UUIDs of the integrations. Use it for `channels` of `healthchecksio_check`.", ElementType: types.StringType, Computed: true},
		},
	}
}

func (d *channelsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg channelsModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	channels, err := d.c.ListChannels(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list the integrations", err.Error())
		return
	}
	cfg.Channels = []channelModel{}
	var ids []string
	for _, ch := range channels {
		if !cfg.Kind.IsNull() && ch.Kind != cfg.Kind.ValueString() || !cfg.Name.IsNull() && ch.Name != cfg.Name.ValueString() {
			continue
		}
		cfg.Channels = append(cfg.Channels, channelModel{types.StringValue(ch.ID), types.StringValue(ch.Name), types.StringValue(ch.Kind)})
		ids = append(ids, ch.ID)
	}
	cfg.IDs = stringSet(ids)
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}

// badgesDataSource gives the badge URLs of each tag.
type badgesDataSource struct{ dataSourceBase }

func newBadgesDataSource() datasource.DataSource {
	return &badgesDataSource{dataSourceBase{name: "badges"}}
}

type badgeModel struct {
	SVG      types.String `tfsdk:"svg"`
	SVG3     types.String `tfsdk:"svg3"`
	JSON     types.String `tfsdk:"json"`
	JSON3    types.String `tfsdk:"json3"`
	Shields  types.String `tfsdk:"shields"`
	Shields3 types.String `tfsdk:"shields3"`
}

type badgesModel struct {
	Badges map[string]badgeModel `tfsdk:"badges"`
}

func (d *badgesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	url := func(desc string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "The badge URLs of each tag of the project. A read-only API key is sufficient. For the badge of one check, use `badge_url` of the check.",
		Attributes: map[string]schema.Attribute{
			"badges": schema.MapNestedAttribute{
				MarkdownDescription: "The badges, by tag. The key `*` is for all checks of the project.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"svg":      url("An SVG badge with two states: up and down. A check in its grace period shows up."),
					"svg3":     url("An SVG badge with three states: up, late and down."),
					"json":     url("A JSON badge with two states."),
					"json3":    url("A JSON badge with three states."),
					"shields":  url("A Shields.io endpoint badge with two states."),
					"shields3": url("A Shields.io endpoint badge with three states."),
				}},
			},
		},
	}
}

func (d *badgesDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	badges, err := d.c.ListBadges(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Cannot list the badges", err.Error())
		return
	}
	out := badgesModel{Badges: make(map[string]badgeModel, len(badges))}
	for tag, b := range badges {
		out.Badges[tag] = badgeModel{
			types.StringValue(b.SVG), types.StringValue(b.SVG3), types.StringValue(b.JSON),
			types.StringValue(b.JSON3), types.StringValue(b.Shields), types.StringValue(b.Shields3),
		}
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &out)...)
}

// statusDataSource reads the status endpoint of the server.
type statusDataSource struct{ dataSourceBase }

func newStatusDataSource() datasource.DataSource {
	return &statusDataSource{dataSourceBase{name: "status"}}
}

type statusModel struct {
	OK types.Bool `tfsdk:"ok"`
}

func (d *statusDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The status of the server. Use it in a precondition for a self-hosted server. If the server does not answer, the read fails.",
		Attributes: map[string]schema.Attribute{
			"ok": schema.BoolAttribute{MarkdownDescription: "True when the server can query its database. False when the server answers with a 5xx status.", Computed: true},
		},
	}
}

// Read sets ok to false for a 5xx status. The server answers 500 when it cannot query
// its database. Any other error stops the read.
func (d *statusDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	err := d.c.Status(ctx)
	var apiErr *client.APIError
	down := errors.As(err, &apiErr) && apiErr.Status >= 500
	if err != nil && !down {
		resp.Diagnostics.AddError("Cannot read the server status", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &statusModel{OK: types.BoolValue(err == nil)})...)
}
