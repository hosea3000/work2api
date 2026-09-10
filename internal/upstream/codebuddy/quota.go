package codebuddy

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// quotaProductCode 个人版额度查询的产品码（对齐参考实现）。
const quotaProductCode = "p_tcaca"

// FetchQuotaPersonal 查询个人版额度：把有效套餐的周期容量求和为 total、周期剩余求和为 remaining。
// 企业版接口不在本期范围。
func (c *Client) FetchQuotaPersonal(ctx context.Context, cred CredentialSnapshot) (total, remaining float64, err error) {
	headers, err := GenerateHeaders(cred, ConversationIDs{}, c.CLIVersion)
	if err != nil {
		return 0, 0, err
	}
	headers["Accept"] = "application/json, text/plain, */*"
	payload := map[string]any{
		"PageNumber":               1,
		"PageSize":                 200,
		"ProductCode":              quotaProductCode,
		"Status":                   []int{0, 3},
		"PackageEndTimeRangeBegin": time.Now().Format("2006-01-02 15:04:05"),
		"PackageEndTimeRangeEnd":   "2127-01-01 00:00:00",
	}
	resp, err := c.doJSON(ctx, http.MethodPost, c.Endpoint+"/v2/billing/meter/get-user-resource", headers, payload, 30*time.Second)
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, 0, handleNon200(resp)
	}
	var body struct {
		Code *int `json:"code"`
		Data *struct {
			Response *struct {
				Data *struct {
					Accounts []map[string]any `json:"Accounts"`
				} `json:"Data"`
			} `json:"Response"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return 0, 0, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "quota response invalid json"}
	}
	if body.Code == nil || *body.Code != 0 || body.Data == nil || body.Data.Response == nil || body.Data.Response.Data == nil {
		return 0, 0, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "quota response invalid payload"}
	}
	for _, acc := range body.Data.Response.Data.Accounts {
		if status, ok := flexibleFloat(acc["Status"]); !ok || status != 0 {
			continue
		}
		size, ok := cycleCapacity(acc, "CycleCapacitySize")
		if !ok {
			return 0, 0, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "quota response invalid capacity"}
		}
		remain, ok := cycleCapacity(acc, "CycleCapacityRemain")
		if !ok {
			return 0, 0, &UpstreamError{StatusCode: 502, ErrType: ErrCategoryInvalidResp, Message: "quota response invalid capacity"}
		}
		total += size
		remaining += remain
	}
	return total, remaining, nil
}

// cycleCapacity 取周期容量，Precise 字段优先。
func cycleCapacity(acc map[string]any, field string) (float64, bool) {
	if v, ok := acc[field+"Precise"]; ok && v != nil {
		return flexibleFloat(v)
	}
	return flexibleFloat(acc[field])
}

// flexibleFloat 接受 JSON 数字或数字字符串。
func flexibleFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}
