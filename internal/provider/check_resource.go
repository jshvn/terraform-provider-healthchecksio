package provider

import (
	"context"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/resourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/identityschema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

var (
	_ resource.ResourceWithConfigure        = (*checkResource)(nil)
	_ resource.ResourceWithImportState      = (*checkResource)(nil)
	_ resource.ResourceWithIdentity         = (*checkResource)(nil)
	_ resource.ResourceWithModifyPlan       = (*checkResource)(nil)
	_ resource.ResourceWithConfigValidators = (*checkResource)(nil)
)

func newCheckResource() resource.Resource { return &checkResource{} }

// checkResource is one check. Its id is the unique_key, not the UUID. The UUID is the
// ping credential, and Terraform shows the id in each plan. The UUID and the ping URL
// are sensitive attributes.
type checkResource struct {
	c *client.Client
}

// checkIdentity is the resource identity. An import block can give it in place of an
// id.
type checkIdentity struct {
	UniqueKey types.String `tfsdk:"unique_key"`
}

func (r *checkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_check"
}

func (r *checkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	r.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *checkResource) IdentitySchema(_ context.Context, _ resource.IdentitySchemaRequest, resp *resource.IdentitySchemaResponse) {
	resp.IdentitySchema = identityschema.Schema{
		Attributes: map[string]identityschema.Attribute{
			"unique_key": identityschema.StringAttribute{
				RequiredForImport: true,
				Description:       "The unique_key of the check: 40 hex digits. It is not a ping credential.",
			},
		},
	}
}

func (r *checkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "A check. The check is simple when it has `timeout`, and cron or OnCalendar when it has `schedule`. " +
			"Each attribute has the name of its API field.",
		Attributes: checkResourceAttributes(),
	}
}

func checkResourceAttributes() map[string]schema.Attribute {
	keepID := []planmodifier.String{stringplanmodifier.UseStateForUnknown()}
	keywords := func(kind string) schema.SetAttribute {
		return schema.SetAttribute{
			MarkdownDescription: "The keywords that make a ping a " + kind + " signal. Use them with the `filter_*` attributes. A keyword cannot contain a comma.",
			ElementType:         types.StringType,
			Optional:            true,
			Computed:            true,
			Default:             setdefault.StaticValue(stringSet(nil)),
			Validators:          []validator.Set{setvalidator.ValueStringsAre(stringvalidator.RegexMatches(regexp.MustCompile(`^[^,]+$`), "must not contain a comma"))},
		}
	}
	filter := func(desc string) schema.BoolAttribute {
		return schema.BoolAttribute{MarkdownDescription: desc, Optional: true, Computed: true, Default: booldefault.StaticBool(false)}
	}
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			MarkdownDescription: "The unique_key of the check: 40 hex digits. Use it to import the check. It is not a ping credential.",
			Computed:            true,
			PlanModifiers:       keepID,
		},
		"uuid":      schema.StringAttribute{MarkdownDescription: "The UUID of the check. It is the ping credential.", Computed: true, Sensitive: true, PlanModifiers: keepID},
		"ping_url":  schema.StringAttribute{MarkdownDescription: "The ping URL. It contains the UUID.", Computed: true, Sensitive: true, PlanModifiers: keepID},
		"badge_url": schema.StringAttribute{MarkdownDescription: "The URL of the 2-state SVG badge of the check. For a JSON or Shields badge, change the extension to `.json` or `.shields`. For a 3-state badge, change `/b/2/` to `/b/3/`.", Computed: true, PlanModifiers: keepID},
		"name":      schema.StringAttribute{MarkdownDescription: "The name of the check.", Required: true, Validators: []validator.String{stringvalidator.LengthAtMost(100)}},
		"slug": schema.StringAttribute{
			MarkdownDescription: "The slug of the check, for a ping URL with the ping key of the project. The API does not make it from the name.",
			Optional:            true, Computed: true, Default: stringdefault.StaticString(""),
			Validators: []validator.String{stringvalidator.LengthAtMost(100), stringvalidator.RegexMatches(regexp.MustCompile(`^[a-z0-9_-]*$`), "must contain only a-z, 0-9, hyphens and underscores")},
		},
		"tags": schema.SetAttribute{
			MarkdownDescription: "The tags of the check. A tag cannot contain a space.",
			ElementType:         types.StringType, Optional: true, Computed: true, Default: setdefault.StaticValue(stringSet(nil)),
			Validators: []validator.Set{setvalidator.ValueStringsAre(stringvalidator.RegexMatches(regexp.MustCompile(`^\S+$`), "must not contain a space"))},
		},
		"desc": schema.StringAttribute{MarkdownDescription: "The description of the check.", Optional: true, Computed: true, Default: stringdefault.StaticString(""), Validators: []validator.String{stringvalidator.LengthAtMost(10000)}},
		"timeout": schema.Int64Attribute{
			MarkdownDescription: "For a simple check: the expected period between pings, in seconds. If the check has no `schedule` and you do not set it, the value is 86400. It is null for a check with a `schedule`.",
			Optional:            true, Computed: true,
			Validators: []validator.Int64{int64validator.Between(minPeriod, maxPeriod)},
		},
		"schedule": schema.StringAttribute{
			MarkdownDescription: "For a cron or OnCalendar check: the cron or systemd OnCalendar expression. The server finds the type of the expression.",
			Optional:            true,
			Validators:          []validator.String{stringvalidator.LengthBetween(1, 100)},
		},
		"tz": schema.StringAttribute{
			MarkdownDescription: "For a check with a `schedule`: the IANA time zone of the schedule. The default is `UTC`. Use the canonical name. If you give a legacy name (for example `Europe/Kiev`), the server stores the canonical name (`Europe/Kyiv`), the apply fails, and the error gives the name to use.",
			Optional:            true, Computed: true,
			Validators: []validator.String{stringvalidator.AlsoRequires(path.MatchRoot("schedule"))},
		},
		"grace": schema.Int64Attribute{
			MarkdownDescription: "The grace period in seconds: the time after a late ping before the check is down.",
			Optional:            true, Computed: true, Default: int64default.StaticInt64(defaultGrace),
			Validators: []validator.Int64{int64validator.Between(minPeriod, maxPeriod)},
		},
		"manual_resume": filter("If true, a paused check ignores pings and stays paused until you resume it. If false, a ping resumes the check."),
		"methods": schema.StringAttribute{
			MarkdownDescription: "The HTTP methods for pings: `\"\"` accepts HEAD, GET and POST, and `\"POST\"` accepts only POST.",
			Optional:            true, Computed: true, Default: stringdefault.StaticString(""),
			Validators: []validator.String{stringvalidator.OneOf("", "POST")},
		},
		"channels": schema.SetAttribute{
			MarkdownDescription: "The UUIDs of the integrations that get the alerts of the check. Get them from `healthchecksio_channels`. " +
				"An empty set removes all integrations. If you do not set this attribute, the provider does not change the integrations of the check.",
			ElementType: types.StringType, Optional: true, Computed: true,
			PlanModifiers: []planmodifier.Set{setplanmodifier.UseStateForUnknown()},
			Validators:    []validator.Set{setvalidator.ValueStringsAre(stringvalidator.RegexMatches(lowerUUIDRE, "must be an integration UUID in lowercase, as the server writes it"))},
		},
		"start_kw":            keywords("start"),
		"success_kw":          keywords("success"),
		"failure_kw":          keywords("failure"),
		"filter_subject":      filter("Look for the keywords in the subject of an email ping."),
		"filter_body":         filter("Look for the keywords in the body of an email ping."),
		"filter_http_body":    filter("Look for the keywords in the body of an HTTP ping."),
		"filter_default_fail": filter("If true, a filtered ping that has no keyword is a failure signal. If false, the server ignores that ping."),
	}
}

func (r *checkResource) ConfigValidators(context.Context) []resource.ConfigValidator {
	return []resource.ConfigValidator{
		resourcevalidator.Conflicting(path.MatchRoot("timeout"), path.MatchRoot("schedule")),
	}
}

// ModifyPlan sets the attributes of the other kind to null. A simple check gets the
// default timeout when it has none. A check with a schedule gets the default tz when it
// has none. When the schedule is unknown, the kind is unknown. Then timeout and tz are
// unknown too.
func (r *checkResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var cfg checkFields
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, tz := cfg.Timeout, cfg.TZ
	switch {
	case cfg.Schedule.IsUnknown():
		timeout, tz = types.Int64Unknown(), types.StringUnknown()
	case cfg.Schedule.IsNull():
		tz = types.StringNull()
		if timeout.IsNull() {
			timeout = types.Int64Value(defaultTimeout)
		}
	default:
		timeout = types.Int64Null()
		if tz.IsNull() {
			tz = types.StringValue(defaultTZ)
		}
	}
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("timeout"), timeout)...)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("tz"), tz)...)
}

func (r *checkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan checkFields
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	spec, diags := plan.spec(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.c.CreateCheck(ctx, spec)
	if err != nil {
		resp.Diagnostics.AddError("Cannot create the check", err.Error())
		return
	}
	if d := tzDiag(plan, got); d != nil {
		resp.Diagnostics.Append(d)
		// The configuration cannot describe the new check. Thus, Create deletes it. If the
		// delete fails, the state gets the check, and Terraform marks it as tainted.
		if err := r.c.DeleteCheck(ctx, got.UUID); err != nil && !client.IsNotFound(err) {
			resp.Diagnostics.AddError("Cannot delete the check", err.Error())
			r.save(ctx, got, &resp.State, resp.Identity, &resp.Diagnostics)
		}
		return
	}
	r.save(ctx, got, &resp.State, resp.Identity, &resp.Diagnostics)
}

func (r *checkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state checkFields
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	uuid := uuidFrom(state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.c.GetCheck(ctx, uuid)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Cannot read the check", err.Error())
		return
	}
	r.save(ctx, got, &resp.State, resp.Identity, &resp.Diagnostics)
}

func (r *checkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state checkFields
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	spec, diags := plan.spec(ctx)
	resp.Diagnostics.Append(diags...)
	uuid := uuidFrom(state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.c.UpdateCheck(ctx, uuid, spec)
	if err != nil {
		resp.Diagnostics.AddError("Cannot update the check", err.Error())
		return
	}
	// After this error, the state stays as it was. The next refresh reads the server.
	if d := tzDiag(plan, got); d != nil {
		resp.Diagnostics.Append(d)
		return
	}
	r.save(ctx, got, &resp.State, resp.Identity, &resp.Diagnostics)
}

// tzDiag gives an error when the server stores a different tz than the plan. The server
// changes a legacy name (for example Europe/Kiev) to the canonical name (Europe/Kyiv),
// and no API gives the canonical name before the apply. Then the state cannot agree
// with the plan. Thus, the apply fails, and the message gives the name to use.
func tzDiag(plan checkFields, got *client.Check) diag.Diagnostic {
	have := types.StringPointerValue(got.TZ)
	if plan.TZ.IsNull() || plan.TZ.IsUnknown() || have.Equal(plan.TZ) {
		return nil
	}
	return diag.NewAttributeErrorDiagnostic(path.Root("tz"), "The server changed tz",
		fmt.Sprintf("The server stores %s in place of %s. Use %s.", have.ValueString(), plan.TZ.ValueString(), have.ValueString()))
}

func (r *checkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state checkFields
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	uuid := uuidFrom(state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.c.DeleteCheck(ctx, uuid); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Cannot delete the check", err.Error())
	}
}

// ImportState accepts a UUID or a unique_key as the id, or the identity. A unique_key
// can go in an import block in git, because it is not a ping credential.
func (r *checkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if id == "" && req.Identity != nil {
		var ident checkIdentity
		resp.Diagnostics.Append(req.Identity.Get(ctx, &ident)...)
		id = ident.UniqueKey.ValueString()
	}
	uuid, err := resolveUUID(ctx, r.c, id)
	if err != nil {
		resp.Diagnostics.AddError("Cannot import the check", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("uuid"), uuid)...)
}

// uuidFrom gives the uuid in the state. A state from another provider has no uuid. With
// no uuid, the path of a request is the list or create endpoint. Thus, uuidFrom adds an
// error, and the caller must stop.
func uuidFrom(state checkFields, diags *diag.Diagnostics) string {
	uuid := state.UUID.ValueString()
	if uuid == "" {
		diags.AddError("The state has no uuid",
			"The state of this check is from a different provider. Remove the check from the state with `terraform state rm`, and import it again with its unique_key.")
	}
	return uuid
}

// save writes the API check to the state and to the identity. identity is nil when the
// client does not support resource identity.
func (r *checkResource) save(ctx context.Context, got *client.Check, state *tfsdk.State, identity *tfsdk.ResourceIdentity, diags *diag.Diagnostics) {
	fields := fieldsFromAPI(got)
	diags.Append(state.Set(ctx, &fields)...)
	if identity != nil {
		diags.Append(identity.Set(ctx, checkIdentity{UniqueKey: fields.ID})...)
	}
}
