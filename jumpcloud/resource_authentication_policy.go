package jumpcloud

import (
	"encoding/json"
	"errors"
	"fmt"

	jcapiv2 "github.com/TheJumpCloud/jcapi-go/v2"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
)

// resourceAuthenticationPolicy manages a JumpCloud authentication policy
// (JumpCloud's conditional-access policy engine, at /authn/policies in the v2
// API). This is the current, documented mechanism for requiring MFA on user
// logins -- confirmed against TheJumpCloud/jc-cli, JumpCloud's own actively
// maintained CLI -- since jcapi-go (this provider's Go SDK dependency, with no
// real code change since 2019) has no knowledge of this endpoint at all.
//
// The conditions tree (who/what the policy applies to) is exposed as a raw
// JSON string rather than a typed schema: jc-cli itself treats it as an opaque
// JSON passthrough with no typed model, and there's no confirmed schema for it
// to build against without guessing.
func resourceAuthenticationPolicy() *schema.Resource {
	return &schema.Resource{
		Description: "Manages a JumpCloud authentication policy (conditional " +
			"access / MFA enforcement). See https://github.com/TheJumpCloud/jc-cli " +
			"for reference -- this provider's underlying Go SDK has no model for " +
			"this endpoint at all.",
		Create: resourceAuthenticationPolicyCreate,
		Read:   resourceAuthenticationPolicyRead,
		Update: resourceAuthenticationPolicyUpdate,
		Delete: resourceAuthenticationPolicyDelete,
		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},
		Schema: map[string]*schema.Schema{
			"name": {
				Description: "Policy name.",
				Type:        schema.TypeString,
				Required:    true,
			},
			"type": {
				Description: "Policy type, e.g. \"user_portal\" or \"admin\".",
				Type:        schema.TypeString,
				Optional:    true,
				Computed:    true,
			},
			"disabled": {
				Description: "Whether the policy is disabled.",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},
			"effect": {
				Description: "The policy's action when it applies: \"allow\", " +
					"\"deny\", or \"allow_with_mfa\" (confirmed via JumpCloud's own " +
					"jc-cli source -- its Policy simulation model documents exactly " +
					"these three values; the API itself returned a 400 " +
					"\"missing effect\" when this field was absent, confirming it's " +
					"required).",
				Type:     schema.TypeString,
				Required: true,
			},
			"target_all_users": {
				Description: "Apply this policy to all users. Without this (or " +
					"target_user_groups) set, a policy matches no one and is " +
					"silently inert even if enabled -- confirmed via jc-cli's own " +
					"policy-evaluation logic.",
				Type:     schema.TypeBool,
				Optional: true,
				Default:  false,
			},
			"target_user_groups": {
				Description: "JumpCloud user group IDs this policy targets.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"target_applications": {
				Description: "JumpCloud application IDs this policy targets.",
				Type:        schema.TypeList,
				Optional:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
			},
			"conditions_json": {
				Description: "The policy's conditions tree, as a raw JSON string. " +
					"No typed schema is exposed for this -- its shape isn't " +
					"confirmed, and JumpCloud's own actively maintained CLI " +
					"(jc-cli) treats it as opaque JSON too. Leave unset for a " +
					"policy with no conditions (applies unconditionally, if " +
					"JumpCloud's API accepts an absent/empty conditions tree -- " +
					"verify this live before relying on it).",
				Type:     schema.TypeString,
				Optional: true,
			},
			"mfa_required": {
				Description: "Require MFA under this policy.",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},
			"mfa_allow_enrollment": {
				Description: "Allow MFA self-enrollment under this policy.",
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     false,
			},
		},
	}
}

func resourceAuthenticationPolicyCreate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*jcapiv2.Configuration)
	apiKey := config.DefaultHeader["x-api-key"]

	body, err := buildAuthPolicyRequestBody(nil, d)
	if err != nil {
		return err
	}

	result, err := AuthPolicyCreateRaw(config.BasePath, apiKey, body)
	if err != nil {
		return err
	}

	id, _ := result["id"].(string)
	if id == "" {
		id, _ = result["_id"].(string)
	}
	if id == "" {
		return fmt.Errorf("authentication policy created but no id was returned: %+v", result)
	}
	d.SetId(id)
	return resourceAuthenticationPolicyRead(d, meta)
}

func resourceAuthenticationPolicyRead(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*jcapiv2.Configuration)
	apiKey := config.DefaultHeader["x-api-key"]

	raw, err := AuthPolicyGetRaw(config.BasePath, apiKey, d.Id())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			d.SetId("")
			return nil
		}
		return err
	}

	if v, ok := raw["name"].(string); ok {
		if err := d.Set("name", v); err != nil {
			return err
		}
	}
	if v, ok := raw["type"].(string); ok {
		if err := d.Set("type", v); err != nil {
			return err
		}
	}
	if v, ok := raw["disabled"].(bool); ok {
		if err := d.Set("disabled", v); err != nil {
			return err
		}
	}
	if v, ok := raw["effect"].(string); ok {
		if err := d.Set("effect", v); err != nil {
			return err
		}
	}
	if targets, ok := raw["targets"].(map[string]interface{}); ok {
		allUsers, _ := targets["allUsers"].(bool)
		if err := d.Set("target_all_users", allUsers); err != nil {
			return err
		}
		if err := d.Set("target_user_groups", stringSliceFromAny(targets["userGroups"])); err != nil {
			return err
		}
		if err := d.Set("target_applications", stringSliceFromAny(targets["applications"])); err != nil {
			return err
		}
	}
	if cond, ok := raw["conditions"]; ok && cond != nil {
		b, err := json.Marshal(cond)
		if err != nil {
			return fmt.Errorf("re-encoding conditions for state: %w", err)
		}
		if err := d.Set("conditions_json", string(b)); err != nil {
			return err
		}
	}
	if mfa, ok := raw["mfa"].(map[string]interface{}); ok {
		required, _ := mfa["required"].(bool)
		if err := d.Set("mfa_required", required); err != nil {
			return err
		}
		allowEnrollment, _ := mfa["allowEnrollment"].(bool)
		if err := d.Set("mfa_allow_enrollment", allowEnrollment); err != nil {
			return err
		}
	}

	return nil
}

func resourceAuthenticationPolicyUpdate(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*jcapiv2.Configuration)
	apiKey := config.DefaultHeader["x-api-key"]

	current, err := AuthPolicyGetRaw(config.BasePath, apiKey, d.Id())
	if err != nil {
		return err
	}
	body, err := buildAuthPolicyRequestBody(current, d)
	if err != nil {
		return err
	}
	if _, err := AuthPolicyUpdateRaw(config.BasePath, apiKey, d.Id(), body); err != nil {
		return err
	}
	return resourceAuthenticationPolicyRead(d, meta)
}

func resourceAuthenticationPolicyDelete(d *schema.ResourceData, meta interface{}) error {
	config := meta.(*jcapiv2.Configuration)
	apiKey := config.DefaultHeader["x-api-key"]

	if err := AuthPolicyDeleteRaw(config.BasePath, apiKey, d.Id()); err != nil {
		if errors.Is(err, ErrNotFound) {
			d.SetId("")
			return nil
		}
		return err
	}
	d.SetId("")
	return nil
}

// buildAuthPolicyRequestBody applies this resource's managed fields onto seed
// (nil for Create, or the policy's current raw state for Update) and returns
// the result as the request body -- the same read-modify-write pattern used
// for jumpcloud_application's Update, since the real policy object may have
// fields this resource doesn't expose.
func buildAuthPolicyRequestBody(seed map[string]interface{}, d *schema.ResourceData) (map[string]interface{}, error) {
	body := seed
	if body == nil {
		body = map[string]interface{}{}
	}

	body["name"] = d.Get("name").(string)
	body["disabled"] = d.Get("disabled").(bool)
	body["effect"] = d.Get("effect").(string)
	if v, ok := d.GetOk("type"); ok {
		body["type"] = v.(string)
	}

	body["targets"] = map[string]interface{}{
		"allUsers":     d.Get("target_all_users").(bool),
		"userGroups":   stringSliceFromInterfaceList(d.Get("target_user_groups").([]interface{})),
		"applications": stringSliceFromInterfaceList(d.Get("target_applications").([]interface{})),
	}

	if v, ok := d.GetOk("conditions_json"); ok {
		var cond interface{}
		if err := json.Unmarshal([]byte(v.(string)), &cond); err != nil {
			return nil, fmt.Errorf("conditions_json is not valid JSON: %w", err)
		}
		body["conditions"] = cond
	}

	body["mfa"] = map[string]interface{}{
		"required":        d.Get("mfa_required").(bool),
		"allowEnrollment": d.Get("mfa_allow_enrollment").(bool),
	}

	return body, nil
}

// stringSliceFromInterfaceList converts a Terraform TypeList of strings
// (as returned by d.Get) into a []string.
func stringSliceFromInterfaceList(raw []interface{}) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

// stringSliceFromAny converts a raw JSON array value (as decoded into
// []interface{} by encoding/json) into a []string, for setting into a
// Terraform TypeList. A missing or non-array value yields an empty slice.
func stringSliceFromAny(raw interface{}) []string {
	arr, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, v := range arr {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
