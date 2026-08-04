package web

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/cartine/thimble/internal/dotenv"
	"github.com/cartine/thimble/internal/passphrase"
	"github.com/cartine/thimble/internal/store"
)

const maxSecretFormBytes = 64 << 10
const maxGeneratedSecrets = 50

func (s *Server) handleSecret(w http.ResponseWriter, r *http.Request) {
	if !s.requireSession(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSecretFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid or oversized secret form", http.StatusBadRequest)
		return
	}
	action := r.FormValue("action")
	if isSecretWriteAction(action) && (!s.loopback || !s.canSet) {
		s.redirectErr(w, r, errors.New(secretSetUnavailable(s.loopback)))
		return
	}
	writtenKeys, err := s.runSecretAction(r, action)
	if err != nil {
		s.redirectErr(w, r, err)
		return
	}
	if isSecretWriteAction(action) {
		q := r.URL.Query()
		q.Set("notice", secretSavedNotice(action, len(writtenKeys)))
		q.Set("app", r.FormValue("app"))
		q.Set("env", r.FormValue("env"))
		q.Del("saved")
		for _, key := range writtenKeys {
			q.Add("saved", key)
		}
		http.Redirect(w, r, "/?"+q.Encode(), http.StatusSeeOther)
		return
	}
	s.redirectNotice(w, r, fmt.Sprintf("%s deleted", r.FormValue("key")))
}

func isSecretWriteAction(action string) bool {
	return action == "set" || action == "generate-passphrase" ||
		action == "generate-passphrases"
}

func secretSavedNotice(action string, count int) string {
	if action == "generate-passphrases" {
		return fmt.Sprintf("%d four-word secrets generated and saved without being displayed", count)
	}
	if action == "generate-passphrase" {
		return "four-word secret generated and saved without being displayed"
	}
	return "secret saved; use the retrieval command below"
}

func (s *Server) runSecretAction(r *http.Request, action string) ([]string, error) {
	app, env, key := r.FormValue("app"), r.FormValue("env"), r.FormValue("key")
	st, _, err := s.stores.Current()
	if err != nil {
		return nil, err
	}
	if st == nil {
		return nil, errors.New("select or create a store first")
	}
	switch action {
	case "set":
		value := r.FormValue("value")
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("empty secret values are not accepted")
		}
		if err := st.SetSecret(app, env, key, value); err != nil {
			return nil, err
		}
		return []string{key}, nil
	case "generate-passphrase":
		if r.FormValue("generator") != passphrase.PresetHyphenated4WGT30C {
			return nil, errors.New("unsupported secret generator")
		}
		value, generateErr := passphrase.Generate()
		if generateErr != nil {
			return nil, generateErr
		}
		if err := st.SetSecret(app, env, key, value); err != nil {
			return nil, err
		}
		return []string{key}, nil
	case "generate-passphrases":
		return generatePassphrases(st, app, env, r.Form["key"], r.FormValue("generator"))
	case "delete":
		return nil, st.DeleteSecret(app, env, key)
	default:
		return nil, errors.New("unknown secret action")
	}
}

func generatePassphrases(
	st *store.Store,
	app, env string,
	rawKeys []string,
	generator string,
) ([]string, error) {
	if generator != passphrase.PresetHyphenated4WGT30C {
		return nil, errors.New("unsupported secret generator")
	}
	keys, err := uniqueSecretKeys(rawKeys)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		values[key], err = passphrase.Generate()
		if err != nil {
			return nil, err
		}
	}
	if err := st.SetSecrets(app, env, values); err != nil {
		return nil, err
	}
	return keys, nil
}

func uniqueSecretKeys(rawKeys []string) ([]string, error) {
	if len(rawKeys) == 0 {
		return nil, errors.New("stage at least one secret key")
	}
	keys := make([]string, 0, len(rawKeys))
	seen := make(map[string]struct{}, len(rawKeys))
	for _, rawKey := range rawKeys {
		key := strings.TrimSpace(rawKey)
		if err := dotenv.ValidateKey(key); err != nil {
			return nil, err
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
		if len(keys) > maxGeneratedSecrets {
			return nil, fmt.Errorf("generate at most %d secrets at a time", maxGeneratedSecrets)
		}
	}
	return keys, nil
}

func secretSetUnavailable(loopback bool) string {
	if !loopback {
		return "web secret entry is available only on a loopback address; use the CLI"
	}
	return "restart thimble web with --identity or THIMBLE_AGE_IDENTITY to set secrets"
}
