/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package hosts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/hosts"
	"github.com/IrineSistiana/mosdns/v5/pkg/matcher/domain"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/go-chi/chi/v5"
	"github.com/miekg/dns"
	"os"
)

const PluginType = "hosts"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() any { return new(Args) })
}

var _ sequence.Executable = (*Hosts)(nil)

type Args struct {
	Entries []string `yaml:"entries"`
	Files   []string `yaml:"files"`
}

type Hosts struct {
	h *hosts.Hosts
	// [热更新] 保存当前 entries（POST /post 重建用）
	entries []string
}

func Init(bp *coremain.BP, args any) (any, error) {
	h, err := NewHosts(args.(*Args))
	if err != nil {
		return nil, err
	}
	// [热更新] 注册 API：POST /plugins/{tag}/post 替换 entries（免重启）
	bp.RegAPI(h.api())
	return h, nil
}

// rebuild 用 entries 重建匹配器（热更新）
func (h *Hosts) rebuild(entries []string) error {
	m := domain.NewMixMatcher[*hosts.IPs]()
	m.SetDefaultMatcher(domain.MatcherFull)
	for i, entry := range entries {
		if err := domain.Load[*hosts.IPs](m, entry, hosts.ParseIPs); err != nil {
			return fmt.Errorf("failed to load entry #%d %s, %w", i, entry, err)
		}
	}
	h.h = hosts.NewHosts(m)
	h.entries = entries
	return nil
}

func (h *Hosts) api() *chi.Mux {
	r := chi.NewRouter()
	r.Post("/post", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Values []string `json:"values"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}
		if err := h.rebuild(body.Values); err != nil {
			http.Error(w, "rebuild failed: "+err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "hosts replaced with %d entries", len(body.Values))
	})
	return r
}

func NewHosts(args *Args) (*Hosts, error) {
	m := domain.NewMixMatcher[*hosts.IPs]()
	m.SetDefaultMatcher(domain.MatcherFull)
	for i, entry := range args.Entries {
		if err := domain.Load[*hosts.IPs](m, entry, hosts.ParseIPs); err != nil {
			return nil, fmt.Errorf("failed to load entry #%d %s, %w", i, entry, err)
		}
	}
	for i, file := range args.Files {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read file #%d %s, %w", i, file, err)
		}
		if err := domain.LoadFromTextReader[*hosts.IPs](m, bytes.NewReader(b), hosts.ParseIPs); err != nil {
			return nil, fmt.Errorf("failed to load file #%d %s, %w", i, file, err)
		}
	}

	return &Hosts{
		h:       hosts.NewHosts(m),
		entries: append([]string(nil), args.Entries...),
	}, nil
}

func (h *Hosts) Response(q *dns.Msg) *dns.Msg {
	return h.h.LookupMsg(q)
}

func (h *Hosts) Exec(_ context.Context, qCtx *query_context.Context) error {
	r := h.h.LookupMsg(qCtx.Q())
	if r != nil {
		qCtx.SetResponse(r)
	}
	return nil
}
