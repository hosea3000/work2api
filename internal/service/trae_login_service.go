package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/yourname/work2api/internal/config"
	"github.com/yourname/work2api/internal/model"
	"github.com/yourname/work2api/internal/repository"
	"github.com/yourname/work2api/internal/upstream/trae"
)

const (
	traePendingTTL   = 10 * time.Minute
	traeCallbackHost = "127.0.0.1"
)

// TraePendingState pending 登录状态。
type TraePendingState string

const (
	TraePendingActive   TraePendingState = "pending"
	TraePendingSuccess  TraePendingState = "success"
	TraePendingFailed   TraePendingState = "failed"
	TraePendingCanceled TraePendingState = "canceled"
)

// TraePendingLogin 单次登录的临时上下文（内存态，重启丢失，符合登录瞬时语义）。
type TraePendingLogin struct {
	State       TraePendingState
	MachineID   string
	DeviceID    string
	CallbackURL string
	CreatedAt   time.Time

	UID      string // success 时填
	Nickname string
	ErrorMsg string // failed 时填
}

// TraeStartResult start 端点返回。
type TraeStartResult struct {
	LoginURL    string `json:"login_url"`
	PendingID   string `json:"pending_id"`
	CallbackURL string `json:"callback_url"`
}

// TraeResultState result 端点返回。
type TraeResultState struct {
	PendingID string           `json:"pending_id"`
	State     TraePendingState `json:"state"`
	UID       string           `json:"uid,omitempty"`
	Nickname  string           `json:"nickname,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// TraeCredentialIngestor 回调捕获与粘贴导入共享的入库流程抽象（供测试替身）。
type TraeCredentialIngestor interface {
	Ingest(ctx context.Context, info *trae.CallbackInfo, machineID, deviceID string) (*model.Credential, error)
}

// TraeLoginService TRAE 网页登录闭环编排。
type TraeLoginService struct {
	mu          sync.Mutex
	logins      map[string]*TraePendingLogin
	callbackURL string // http://127.0.0.1:{port}/authorize

	repo             repository.GatewayRepository
	creds            CredentialService
	client           *trae.Client
	poolOrchestrator *CredentialPool
}

func NewTraeLoginService(repo repository.GatewayRepository, creds CredentialService, client *trae.Client, pool *CredentialPool, conf *config.CodeBuddyConfig) *TraeLoginService {
	port := 18080
	if conf != nil && conf.TraeCallbackPort > 0 {
		port = conf.TraeCallbackPort
	}
	return &TraeLoginService{
		logins:           map[string]*TraePendingLogin{},
		callbackURL:      fmt.Sprintf("http://%s:%d/authorize", traeCallbackHost, port),
		repo:             repo,
		creds:            creds,
		client:           client,
		poolOrchestrator: pool,
	}
}

// Start 生成登录链接 + pending 态。
func (s *TraeLoginService) Start() (*TraeStartResult, error) {
	machineID, err := randomHexTrae(16)
	if err != nil {
		return nil, err
	}
	deviceID, err := randomHexTrae(16)
	if err != nil {
		return nil, err
	}
	pendingID, err := randomHexTrae(8)
	if err != nil {
		return nil, err
	}
	pl := &TraePendingLogin{
		State:       TraePendingActive,
		MachineID:   machineID,
		DeviceID:    deviceID,
		CallbackURL: s.callbackURL,
		CreatedAt:   time.Now(),
	}
	s.mu.Lock()
	s.pruneLocked()
	s.logins[pendingID] = pl
	s.mu.Unlock()
	return &TraeStartResult{
		LoginURL:    trae.BuildLoginURL(machineID, deviceID, s.callbackURL),
		PendingID:   pendingID,
		CallbackURL: s.callbackURL,
	}, nil
}

// Result 查询 pending 状态；不存在或已过期返回 false（→ 404）。
func (s *TraeLoginService) Result(pendingID string) (*TraeResultState, bool) {
	if pendingID == "" {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	pl, ok := s.logins[pendingID]
	if !ok {
		return nil, false
	}
	if time.Since(pl.CreatedAt) > traePendingTTL && pl.State == TraePendingActive {
		delete(s.logins, pendingID)
		return nil, false
	}
	r := &TraeResultState{
		PendingID: pendingID,
		State:     pl.State,
		UID:       pl.UID,
		Nickname:  pl.Nickname,
		Error:     pl.ErrorMsg,
	}
	return r, true
}

// Cancel 取消：删除 pending（取消后到达的回调因无 pending 被拒绝）。
func (s *TraeLoginService) Cancel(pendingID string) bool {
	if pendingID == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.logins[pendingID]
	delete(s.logins, pendingID)
	return ok
}

// HandleAuthorize 处理 /authorize 回调（自动捕获路径）：
// 用 loginTraceID 反查 pending（TRAE 回调不回传 machine/device id），
// 完成入库流程并更新 pending 状态。返回错误供渲染错误页。
func (s *TraeLoginService) HandleAuthorize(ctx context.Context, rawURL string) error {
	info, err := trae.ParseCallback(rawURL)
	if err != nil {
		return err
	}
	traceID := queryParam(rawURL, "loginTraceID")
	pl := s.takePendingByTrace(traceID)
	if pl == nil {
		return fmt.Errorf("no pending login matches this callback (expired or unknown)")
	}
	return s.ingestAndMark(ctx, info, pl)
}

// Import 粘贴回调 URL 导入（远程部署兜底，SessionAuth 保护）。
// machine/device 取回调参数或新生成（无 pending 上下文）。
func (s *TraeLoginService) Import(ctx context.Context, callbackURL string) (*model.Credential, error) {
	info, err := trae.ParseCallback(callbackURL)
	if err != nil {
		return nil, err
	}
	machineID := queryParam(callbackURL, "machine_id")
	deviceID := queryParam(callbackURL, "device_id")
	if machineID == "" || deviceID == "" {
		if machineID, err = randomHexTrae(16); err != nil {
			return nil, err
		}
		if deviceID, err = randomHexTrae(16); err != nil {
			return nil, err
		}
	}
	return s.ingest(ctx, info, machineID, deviceID)
}

// ingestAndMark pending 路径的入库 + 状态标记。
func (s *TraeLoginService) ingestAndMark(ctx context.Context, info *trae.CallbackInfo, pl *TraePendingLogin) error {
	cred, err := s.ingest(ctx, info, pl.MachineID, pl.DeviceID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		pl.State = TraePendingFailed
		pl.ErrorMsg = err.Error()
		return err
	}
	pl.Nickname = derefStr(cred.Nickname)
	pl.UID = cred.UserId
	pl.State = TraePendingSuccess
	return nil
}

// ingest 共享入库流程：ExchangeToken（轮换 RT）→ GetUserInfo → CreateCredential(provider=trae)。
// RefreshToken 缺失（仅 userJwt.Token 兜底）时跳过 Exchange 直接入库。
func (s *TraeLoginService) ingest(ctx context.Context, info *trae.CallbackInfo, machineID, deviceID string) (*model.Credential, error) {
	bearer := info.AccessToken
	refreshToken := info.RefreshToken
	expiresAt := info.ExpiresAt
	if refreshToken != "" {
		pair, err := s.client.ExchangeToken(refreshToken, "")
		if err != nil {
			return nil, fmt.Errorf("exchange token failed: %w", err)
		}
		bearer = pair.AccessToken
		if pair.RefreshToken != "" {
			refreshToken = pair.RefreshToken // 轮换：保留新值
		}
		expiresAt = pair.ExpiresAt
	}
	if bearer == "" {
		return nil, fmt.Errorf("no access token after exchange")
	}

	cred := &model.Credential{
		Id:           newUUID(),
		BearerToken:  bearer,
		AuthSource:   "web_login",
		Provider:     "trae",
		Status:       "active",
		ExpiresAt:    int64Ptr(expiresAt),
		RefreshToken: stringPtr(refreshToken),
		MachineID:    stringPtr(machineID),
		DeviceID:     stringPtr(deviceID),
		UserId:       info.UID,
		Domain:       stringPtr(trae.Domain),
		EnterpriseId: stringPtr(info.EnterpriseID),
		Nickname:     stringPtr(info.Nickname),
	}
	// GetUserInfo 补全 uid/nickname（失败不阻断：回调 userInfo 通常已带）
	if ui, err := s.client.GetUserInfo(bearer, ""); err == nil && ui != nil {
		if ui.UID != "" {
			cred.UserId = ui.UID
		}
		if ui.Nickname != "" {
			cred.Nickname = stringPtr(ui.Nickname)
		}
		if ui.EnterpriseID != "" {
			cred.EnterpriseId = stringPtr(ui.EnterpriseID)
		}
	}
	if cred.UserId == "" {
		return nil, fmt.Errorf("cannot determine uid from callback or GetUserInfo")
	}
	// 同账号去重：同 uid 且同 provider 的凭证已存在则原位更新（重新登录续命），不新增。
	if existing, err := s.repo.GetCredentialByUserId(ctx, cred.UserId); err == nil && existing != nil &&
		existing.Provider == "trae" {
		existing.BearerToken = cred.BearerToken
		existing.RefreshToken = cred.RefreshToken
		existing.ExpiresAt = cred.ExpiresAt
		existing.Status = "active" // 重新登录视为复活
		existing.MachineID = cred.MachineID
		existing.DeviceID = cred.DeviceID
		existing.Nickname = cred.Nickname
		existing.EnterpriseId = cred.EnterpriseId
		if err := s.repo.UpdateCredential(ctx, existing); err != nil {
			return nil, fmt.Errorf("update credential failed: %w", err)
		}
		return existing, nil
	}
	if err := s.repo.CreateCredential(ctx, cred); err != nil {
		return nil, fmt.Errorf("save credential failed: %w", err)
	}
	return cred, nil
}

// takePendingByTrace 用 loginTraceID 反查并保持 pending（不删除，成功/失败都要可见）。
// 找不到返回 nil。
func (s *TraeLoginService) takePendingByTrace(traceID string) *TraePendingLogin {
	if traceID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	for _, pl := range s.logins {
		if pl.State == TraePendingActive && trae.MachineTraceID(pl.MachineID, pl.DeviceID) == traceID {
			return pl
		}
	}
	return nil
}

// pruneLocked 清理过期 pending（调用方持锁）。
func (s *TraeLoginService) pruneLocked() {
	now := time.Now()
	for id, pl := range s.logins {
		if now.Sub(pl.CreatedAt) > traePendingTTL {
			delete(s.logins, id)
		}
	}
}

func queryParam(rawURL, key string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Query().Get(key)
}

func randomHexTrae(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// newUUID 生成 RFC4122 v4 形式 UUID（对齐项目 uuid.NewString() 的存储格式）。
func newUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:32]
}

func int64Ptr(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}
