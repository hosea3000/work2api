package trae

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CheckinResult CheckinClaim 响应解析结果。
type CheckinResult struct {
	Success bool
	Code    *int
	Message string
}

// ugHeaders 设置签到/积分（api.trae.cn）所需头，对齐 trae2api-web UgHeaders。
func ugHeaders(req *http.Request, accessToken, deviceID string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Trae/"+IdeVersion)
	req.Header.Set("Authorization", "Cloud-IDE-JWT "+accessToken)
	req.Header.Set("X-User-Region", "CN")
	if deviceID != "" {
		req.Header.Set("X-Device-Id", deviceID)
	}
}

// CheckinStatus 查询签到状态（幂等闸门：checked_in=true 时无需 claim）。
func (c *Client) CheckinStatus(ctx context.Context, accessToken, deviceID string) (checkedIn bool, credits int64, enable bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.UgHost+EpCheckinStatus, bytes.NewReader([]byte("{}")))
	if err != nil {
		return false, 0, false, err
	}
	ugHeaders(req, accessToken, deviceID)
	data, err := c.doJSONContext(ctx, req)
	if err != nil {
		return false, 0, false, err
	}
	var resp struct {
		CheckedIn bool  `json:"checked_in"`
		Credits   int64 `json:"credits"`
		Enable    bool  `json:"enable"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return false, 0, false, fmt.Errorf("checkin status parse: %w", err)
	}
	return resp.CheckedIn, resp.Credits, resp.Enable, nil
}

// CheckinClaim 执行签到领取。解析响应 {code, message}，code==0 算成功。
// ponytail: ug 域 claim 响应 schema 未实测（按 ug 域惯例推断）；实测不符时仅改此解析，
// 端点/头不动，极端情况降级为 HTTP 200 即成功（对齐 trae2api-web 不解析体的行为）。
func (c *Client) CheckinClaim(ctx context.Context, accessToken, deviceID string) (*CheckinResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.UgHost+EpCheckinClaim, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, err
	}
	ugHeaders(req, accessToken, deviceID)
	data, err := c.doJSONContext(ctx, req)
	if err != nil {
		return nil, err
	}
	res := &CheckinResult{Success: true, Message: "签到成功"}
	var body struct {
		Code    *int   `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &body) == nil && body.Code != nil {
		res.Code = body.Code
		res.Success = *body.Code == 0
		if body.Message != "" {
			res.Message = body.Message
		}
	}
	return res, nil
}

// EntUsage 查询账号权益额度（聚合 entitlement 包：credits_limit 求和 − usage.credits_amount 已用）。
// 注意：聚合含 work 包，数字仅作权益展示参考，不代表 SOLO 通道真实可用额度。
func (c *Client) EntUsage(ctx context.Context, accessToken, deviceID string) (remain, limit, used int64, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.UgHost+EpEntUsage, bytes.NewReader([]byte("{}")))
	if err != nil {
		return 0, 0, 0, err
	}
	ugHeaders(req, accessToken, deviceID)
	data, err := c.doJSONContext(ctx, req)
	if err != nil {
		return 0, 0, 0, err
	}
	var resp struct {
		UserEntitlementPackList []struct {
			EntitlementBaseInfo struct {
				Quota struct {
					CreditsLimit int64 `json:"credits_limit"`
				} `json:"quota"`
			} `json:"entitlement_base_info"`
			Usage struct {
				CreditsAmount float64 `json:"credits_amount"`
			} `json:"usage"`
		} `json:"user_entitlement_pack_list"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, 0, 0, fmt.Errorf("ent usage parse: %w", err)
	}
	for _, p := range resp.UserEntitlementPackList {
		l := p.EntitlementBaseInfo.Quota.CreditsLimit
		if l <= 0 {
			continue
		}
		u := int64(p.Usage.CreditsAmount)
		limit += l
		used += u
		remain += l - u
	}
	return remain, limit, used, nil
}

// doJSONContext 带请求上下文的 doJSON（HTTP 层超时由 Client 兜底）。
func (c *Client) doJSONContext(_ context.Context, req *http.Request) (json.RawMessage, error) {
	return c.doJSON(req)
}
