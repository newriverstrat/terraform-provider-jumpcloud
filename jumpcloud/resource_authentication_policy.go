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
	if v, ok := d.GetOk("type"); ok {
		body["type"] = v.(string)
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
