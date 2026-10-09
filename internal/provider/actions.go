package provider

import (
	"context"
	"errors"
	"net/http"

	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/jshvn/terraform-provider-healthchecksio/internal/client"
)

// checkAction is the pause or the resume action. The two actions are operations, not
// state: a ping resumes a paused check unless manual_resume is true. Thus, the resource
// has no "paused" attribute. Actions need Terraform 1.14 or later. OpenTofu does not
// support actions.
type checkAction struct {
	name string
	desc string
	run  func(ctx context.Context, c *client.Client, uuid string) (string, error)
	c    *client.Client
}

type checkActionModel struct {
	CheckID types.String `tfsdk:"check_id"`
}

var _ action.ActionWithConfigure = (*checkAction)(nil)

func newPauseAction() action.Action {
	return &checkAction{
		name: "pause",
		desc: "Pauses a check. A paused check sends no alerts. It stays paused until its next ping, or until a resume when `manual_resume` is true. Needs a read-write API key.",
		run: func(ctx context.Context, c *client.Client, uuid string) (string, error) {
			_, err := c.PauseCheck(ctx, uuid)
			return "The check is paused.", err
		},
	}
}

func newResumeAction() action.Action {
	return &checkAction{
		name: "resume",
		desc: "Resumes a paused check. If the check is not paused, the action does nothing. Needs a read-write API key.",
		run: func(ctx context.Context, c *client.Client, uuid string) (string, error) {
			_, err := c.ResumeCheck(ctx, uuid)
			var apiErr *client.APIError
			if errors.As(err, &apiErr) && apiErr.Status == http.StatusConflict {
				return "The check is not paused. No change.", nil
			}
			return "The check is resumed.", err
		},
	}
}

func (a *checkAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + a.name
}

func (a *checkAction) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	a.c = clientFrom(req.ProviderData, &resp.Diagnostics)
}

func (a *checkAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: a.desc,
		Attributes: map[string]schema.Attribute{
			"check_id": schema.StringAttribute{
				MarkdownDescription: "The check: its unique_key (the `id` of `healthchecksio_check`) or its UUID.",
				Required:            true,
			},
		},
	}
}

func (a *checkAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var cfg checkActionModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	uuid, err := resolveUUID(ctx, a.c, cfg.CheckID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Cannot find the check", err.Error())
		return
	}
	msg, err := a.run(ctx, a.c, uuid)
	if err != nil {
		resp.Diagnostics.AddError("Cannot "+a.name+" the check", err.Error())
		return
	}
	resp.SendProgress(action.InvokeProgressEvent{Message: msg})
}
