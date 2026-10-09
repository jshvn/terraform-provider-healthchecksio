package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// checkData is a check with its status fields. The status fields change with each
// ping. Thus, only the data sources have them, not the resource.
type checkData struct {
	checkFields
	Status       types.String `tfsdk:"status"`
	Started      types.Bool   `tfsdk:"started"`
	NPings       types.Int64  `tfsdk:"n_pings"`
	LastPing     types.String `tfsdk:"last_ping"`
	NextPing     types.String `tfsdk:"next_ping"`
	LastDuration types.Int64  `tfsdk:"last_duration"`
}

func dataFromAPI(c *client.Check) checkData {
	return checkData{
		checkFields:  fieldsFromAPI(c),
		Status:       types.StringValue(c.Status),
		Started:      types.BoolValue(c.Started),
		NPings:       types.Int64Value(c.NPings),
		LastPing:     types.StringPointerValue(c.LastPing),
		NextPing:     types.StringPointerValue(c.NextPing),
		LastDuration: types.Int64PointerValue(c.LastDuration),
	}
}

// checkDataAttributes gives the computed attributes of checkData.
func checkDataAttributes() map[string]schema.Attribute {
	str := func(desc string) schema.Attribute {
		return schema.StringAttribute{MarkdownDescription: desc, Computed: true}
	}
	set := func(desc string) schema.Attribute {
		return schema.SetAttribute{MarkdownDescription: desc, ElementType: types.StringType, Computed: true}
	}
	num := func(desc string) schema.Attribute {
		return schema.Int64Attribute{MarkdownDescription: desc, Computed: true}
	}
	flag := func(desc string) schema.Attribute {
		return schema.BoolAttribute{MarkdownDescription: desc, Computed: true}
	}
	return map[string]schema.Attribute{
		"id":                  str("The unique_key of the check."),
		"uuid":                schema.StringAttribute{MarkdownDescription: "The UUID of the check. Null with a read-only key.", Computed: true, Sensitive: true},
		"ping_url":            schema.StringAttribute{MarkdownDescription: "The ping URL. Null with a read-only key.", Computed: true, Sensitive: true},
		"badge_url":           str("The URL of the 2-state SVG badge of the check."),
		"name":                str("The name of the check."),
		"slug":                str("The slug of the check."),
		"tags":                set("The tags of the check."),
		"desc":                str("The description of the check."),
		"timeout":             num("The period of a simple check, in seconds. Null for a check with a schedule."),
		"schedule":            str("The cron or OnCalendar expression. Null for a simple check."),
		"tz":                  str("The time zone of the schedule. Null for a simple check."),
		"grace":               num("The grace period, in seconds."),
		"manual_resume":       flag("If true, a paused check ignores pings until you resume it."),
		"methods":             str("The HTTP methods for pings: \"\" or \"POST\"."),
		"channels":            set("The UUIDs of the integrations of the check. Null with a read-only key."),
		"start_kw":            set("The start keywords."),
		"success_kw":          set("The success keywords."),
		"failure_kw":          set("The failure keywords."),
		"filter_subject":      flag("Look for the keywords in the subject of an email ping."),
		"filter_body":         flag("Look for the keywords in the body of an email ping."),
		"filter_http_body":    flag("Look for the keywords in the body of an HTTP ping."),
		"filter_default_fail": flag("A filtered ping that has no keyword is a failure signal."),
		"status":              str("The status: new, up, grace, down or paused."),
		"started":             flag("True when the check got a start signal and waits for the next signal."),
		"n_pings":             num("The number of pings that the check got."),
		"last_ping":           str("The time of the last ping (RFC 3339), or null."),
		"next_ping":           str("The time when the grace period starts (RFC 3339), or null."),
		"last_duration":       num("The duration of the last job in seconds, from its start signal to its next signal, or null."),
	}
}

var _ datasource.DataSourceWithConfigValidators = (*checkDataSource)(nil)

func newCheckDataSource() datasource.DataSource {
	return &checkDataSource{dataSourceBase{name: "check"}}
}

// checkDataSource finds one check by unique_key or by slug.
type checkDataSource struct{ dataSourceBase }

func (d *checkDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	attrs := checkDataAttributes()
	attrs["id"] = schema.StringAttribute{
		MarkdownDescription: "The unique_key of the check (40 hex digits), not its UUID. Set `id` or `slug`.",
		Optional:            true, Computed: true,
		Validators: []validator.String{stringvalidator.RegexMatches(uniqueKeyRE, "must be a unique_key: 40 lowercase hex digits, not a UUID")},
	}
	attrs["slug"] = schema.StringAttribute{MarkdownDescription: "The slug of the check. Set `id` or `slug`. One check only must have the slug.", Optional: true, Computed: true}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One check, with its status. A read-only API key is sufficient.",
		Attributes:          attrs,
	}
}

func (d *checkDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("slug"))}
}

func (d *checkDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg checkData
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var got *client.Check
	var err error
	if id := cfg.ID.ValueString(); id != "" {
		got, err = d.c.GetCheck(ctx, id)
	} else {
		got, err = d.oneBySlug(ctx, cfg.Slug.ValueString())
	}
	if err != nil {
		resp.Diagnostics.AddError("Cannot read the check", err.Error())
		return
	}
	data := dataFromAPI(got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

func (d *checkDataSource) oneBySlug(ctx context.Context, slug string) (*client.Check, error) {
	checks, err := d.c.ListChecks(ctx, client.CheckFilter{Slug: slug})
	if err != nil {
		return nil, err
	}
	if len(checks) != 1 {
		return nil, fmt.Errorf("want one check with the slug %q, found %d", slug, len(checks))
	}
	return &checks[0], nil
}

func newChecksDataSource() datasource.DataSource {
	return &checksDataSource{dataSourceBase{name: "checks"}}
}

// checksDataSource lists the checks of the project, with an optional filter.
type checksDataSource struct{ dataSourceBase }

type checksModel struct {
	Slug   types.String `tfsdk:"slug"`
	Tags   types.Set    `tfsdk:"tags"`
	Checks []checkData  `tfsdk:"checks"`
}

func (d *checksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "The checks of the project, with their status. A read-only API key is sufficient.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{MarkdownDescription: "Give only the checks with this slug.", Optional: true},
			"tags": schema.SetAttribute{MarkdownDescription: "Give only the checks that have all of these tags.", ElementType: types.StringType, Optional: true},
			"checks": schema.ListNestedAttribute{
				MarkdownDescription: "The checks, in the order of the API.",
				Computed:            true,
				NestedObject:        schema.NestedAttributeObject{Attributes: checkDataAttributes()},
			},
		},
	}
}

func (d *checksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var cfg checksModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	var tags []string
	resp.Diagnostics.Append(cfg.Tags.ElementsAs(ctx, &tags, true)...)
	if resp.Diagnostics.HasError() {
		return
	}
	checks, err := d.c.ListChecks(ctx, client.CheckFilter{Slug: cfg.Slug.ValueString(), Tags: tags})
	if err != nil {
		resp.Diagnostics.AddError("Cannot list the checks", err.Error())
		return
	}
	cfg.Checks = make([]checkData, 0, len(checks))
	for i := range checks {
		cfg.Checks = append(cfg.Checks, dataFromAPI(&checks[i]))
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &cfg)...)
}
