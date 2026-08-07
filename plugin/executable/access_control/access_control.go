/*
 * access_control - 家庭网络访问控制模块
 * 功能：集中管理 blocklist 规则（模式/时段/内容），内置时段匹配自动切换。
 *   MODE=always  全天应用控制规则
 *   MODE=hours   仅 CONTROL_HOURS 时段内应用控制规则（支持跨天如 22:00-07:00）
 *   MODE=off     不应用控制规则（仅系统规则）
 * API:
 *   GET  /plugins/access_control/  状态
 *   POST /plugins/access_control/  {"mode":"hours","control_hours":"22:00-07:00"}
 */
package access_control

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/go-chi/chi/v5"
)

const PluginType = "access_control"

type Args struct {
	// 配置文件路径（POST 保存用）
	ConfigFile string `yaml:"config_file"`
	// 系统规则源文件（始终生效，如防误解析）
	SysRulesFile string `yaml:"sys_rules_file"`
	// 控制规则源文件（时段内生效，如短视频/游戏域名）
	CtrlRulesFile string `yaml:"ctrl_rules_file"`
	// blocklist 目标文件（domain_set_light 的 ruleFile）
	BlocklistFile string `yaml:"blocklist_file"`
	// blocklist 插件的 API tag（用于热更新，默认 blocklist）
	BlocklistTag string `yaml:"blocklist_tag"`
	// mosdns API 地址（热更新自调用）
	ApiAddr string `yaml:"api_addr"`
}

type Config struct {
	Mode          string `json:"mode"`
	ControlHours  string `json:"control_hours"`
}

type AccessControl struct {
	args *Args
	mu   sync.RWMutex
	cfg  Config
}

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

func Init(bp *coremain.BP, args any) (any, error) {
	a := args.(*Args)
	if a.BlocklistTag == "" {
		a.BlocklistTag = "blocklist"
	}
	if a.ApiAddr == "" {
		a.ApiAddr = "127.0.0.1:9099"
	}
	ac := &AccessControl{args: a}

	// 加载持久化配置（无则默认 hours 22:00-07:00）
	if err := ac.loadConfig(); err != nil {
		log.Printf("[access_control] config load failed: %v, use defaults", err)
		ac.cfg = Config{Mode: "hours", ControlHours: "22:00-07:00"}
	}

	// 启动后延迟应用（等 API 就绪）
	go func() {
		time.Sleep(8 * time.Second)
		ac.Apply()
		// 每分钟检查时段变化
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		lastState := ac.inHoursNow()
		for range ticker.C {
			cur := ac.inHoursNow()
			if cur != lastState {
				lastState = cur
				log.Printf("[access_control] period changed, re-applying rules")
				ac.Apply()
			}
		}
	}()

	bp.RegAPI(ac.api())
	return ac, nil
}

// 设备系统时区为 UTC，固定使用北京时区（UTC+8）判断时段
var beijingTZ = time.FixedZone("CST", 8*3600)

func (ac *AccessControl) nowHM() string {
	return time.Now().In(beijingTZ).Format("1504")
}

func (ac *AccessControl) inHoursNow() bool {
	ac.mu.RLock()
	hours := ac.cfg.ControlHours
	ac.mu.RUnlock()
	if hours == "" {
		return false
	}
	parts := strings.SplitN(hours, "-", 2)
	if len(parts) != 2 {
		return false
	}
	start := strings.ReplaceAll(strings.TrimSpace(parts[0]), ":", "")
	end := strings.ReplaceAll(strings.TrimSpace(parts[1]), ":", "")
	now := ac.nowHM()
	if start < end {
		return now >= start && now < end
	}
	return now >= start || now < end
}

func (ac *AccessControl) shouldApply() bool {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	switch ac.cfg.Mode {
	case "always":
		return true
	case "off":
		return false
	default: // hours
		return ac.inHoursNow()
	}
}

// Apply 生成 blocklist 内容（系统规则 + 时段内控制规则）并热更新
func (ac *AccessControl) Apply() error {
	ac.mu.RLock()
	mode := ac.cfg.Mode
	ac.mu.RUnlock()
	apply := mode == "always" || (mode == "hours" && ac.inHoursNow())

	var rules []string
	if sysRules, err := readLines(ac.args.SysRulesFile); err == nil {
		rules = append(rules, sysRules...)
	}
	if apply {
		if ctrlRules, err := readLines(ac.args.CtrlRulesFile); err == nil {
			rules = append(rules, ctrlRules...)
		}
	}

	// 写文件（供重启后 domain_set_light 启动加载）
	if err := writeLines(ac.args.BlocklistFile, rules); err != nil {
		log.Printf("[access_control] ERROR write blocklist: %v", err)
		return err
	}

	// 热更新：POST /plugins/{tag}/post（notifySubscribers 触发 domain_mapper 重建）
	payload, _ := json.Marshal(map[string][]string{"values": rules})
	url := fmt.Sprintf("http://%s/plugins/%s/post", ac.args.ApiAddr, ac.args.BlocklistTag)
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		log.Printf("[access_control] ERROR hot update %s: %v", url, err)
		return err
	}
	resp.Body.Close()
	log.Printf("[access_control] applied: mode=%s in_hours=%v rules=%d", mode, ac.inHoursNow(), len(rules))
	return nil
}

func (ac *AccessControl) loadConfig() error {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	data, err := os.ReadFile(ac.args.ConfigFile)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &ac.cfg)
}

func (ac *AccessControl) saveConfig() error {
	ac.mu.RLock()
	cfg := ac.cfg
	ac.mu.RUnlock()
	data, _ := json.MarshalIndent(cfg, "", "  ")
	if err := os.MkdirAll(filepath.Dir(ac.args.ConfigFile), 0755); err != nil {
		return err
	}
	tmp := ac.args.ConfigFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, ac.args.ConfigFile)
}

func (ac *AccessControl) status() map[string]any {
	ac.mu.RLock()
	mode := ac.cfg.Mode
	hours := ac.cfg.ControlHours
	ac.mu.RUnlock()
	inHours := ac.inHoursNow()
	applyNow := mode == "always" || (mode == "hours" && inHours)
	return map[string]any{
		"mode":          mode,
		"control_hours": hours,
		"now":           time.Now().In(beijingTZ).Format("15:04"),
		"in_hours":      inHours,
		"applying":      applyNow,
	}
}

func (ac *AccessControl) api() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ac.status())
	})
	r.Post("/", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Mode         string `json:"mode"`
			ControlHours string `json:"control_hours"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if body.Mode != "" {
			ac.mu.Lock()
			ac.cfg.Mode = body.Mode
			ac.mu.Unlock()
		}
		if body.ControlHours != "" {
			ac.mu.Lock()
			ac.cfg.ControlHours = body.ControlHours
			ac.mu.Unlock()
		}
		_ = ac.saveConfig()
		if err := ac.Apply(); err != nil {
			http.Error(w, "apply failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(ac.status())
	})
	return r
}

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out, nil
}

func writeLines(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\n")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
