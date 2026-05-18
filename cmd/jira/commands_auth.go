package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/cmdutil"
	"github.com/matcra587/jira-cli/internal/config"
	"github.com/matcra587/jira-cli/internal/jira"
	"github.com/spf13/cobra"
)

func authCommand() *cobra.Command {
	cmd := groupCommand("auth", "Manage Jira authentication", "configuration")
	cmd.AddCommand(authLoginCommand())
	cmd.AddCommand(authStatusCommand())
	cmd.AddCommand(authLogoutCommand())
	cmd.AddCommand(authSwitchCommand())
	cmd.AddCommand(authRefreshCommand())
	cmd.AddCommand(authTokenCommand())
	cmd.AddCommand(authMigrateCommand())
	cmd.AddCommand(authWhoamiCommand())
	return cmd
}

// authWhoamiCommand fetches /myself for the active profile and (optionally)
// caches the resolved accountId in the profile config so subsequent
// `--assignee me` / TUI `A` calls use the canonical Jira identifier.
func authWhoamiCommand() *cobra.Command {
	var save bool
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user's identity from /myself",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Without --save, fetch /myself for the env-overlaid active
			// profile (a read-only view). With --save, both the live
			// request and the persisted record must come from the same
			// file-backed profile, so resolve and Save that profile here
			// and build the client from it: a JIRA_PROFILE_*_BASE_URL
			// overlay must not redirect the request to a different tenant
			// whose account_id would then be written into the file.
			var cfg *config.Config
			var profile config.Profile
			var clientErr error
			var client *jira.Client
			var ok bool
			if save {
				var loadErr error
				cfg, loadErr = config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
				if loadErr != nil {
					return loadErr
				}
				resolved, resolveErr := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
				if resolveErr != nil {
					if errors.Is(resolveErr, config.ErrProfileNotDefined) {
						return fmt.Errorf("validation: cannot --save profile %q: it is not defined in the config file (it exists only via a JIRA_* env overlay)", cmdutil.RequestedProfile(cmd))
					}
					return resolveErr
				}
				// A base_url env overlay would point the request — and the
				// profile's credential — at a tenant other than the
				// file-backed one. --save must operate only on a purely
				// file-backed profile, so refuse before any request.
				if envVar := profileBaseURLEnvVar(resolved.Name); os.Getenv(envVar) != "" {
					return fmt.Errorf("validation: cannot --save profile %q while %s is set: unset that environment variable so --save targets the file-backed Jira tenant", resolved.Name, envVar)
				}
				client, profile, ok, clientErr = cmdutil.JiraClientForProfile(cmd, resolved)
			} else {
				client, profile, ok, clientErr = cmdutil.JiraClientForCommand(cmd)
			}
			if clientErr != nil {
				return clientErr
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for auth.whoami")
			}
			user, _, err := jira.NewUserService(client).Myself(cmd.Context())
			if err != nil {
				return err
			}
			data := map[string]any{
				"profile":       profile.Name,
				"account_id":    user.AccountID,
				"account_type":  user.AccountType,
				"display_name":  user.DisplayName,
				"email_address": user.EmailAddress,
				"time_zone":     user.TimeZone,
				"saved":         false,
			}
			if save {
				saved := false
				for i := range cfg.Profiles {
					if cfg.Profiles[i].Name == profile.Name {
						cfg.Profiles[i].AccountID = user.AccountID
						if cfg.Profiles[i].Email == "" && user.EmailAddress != "" {
							cfg.Profiles[i].Email = user.EmailAddress
						}
						saved = true
						break
					}
				}
				if !saved {
					return fmt.Errorf("validation: cannot --save profile %q: it is not defined in the config file (it exists only via a JIRA_* env overlay)", profile.Name)
				}
				if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
					return err
				}
				data["saved"] = true
			}
			return cmdutil.WriteEnvelope(cmd, "auth.whoami", data)
		},
	}
	cmd.Flags().BoolVar(&save, "save", false, "Persist the resolved account_id (and email if blank) to the active profile")
	return cmd
}

func authLoginCommand() *cobra.Command {
	var profileName, baseURL, authType, email, username, backend, onePasswordAccount, vault, item, credential, credentialEnv, jsonInput string
	var secretStdin bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Configure authentication for a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			noInput := noInputRequested(cmd)
			if !noInput {
				if cmdutil.DetectorFromContext(cmd).Mode != cli.ModePlain && cmdutil.DetectorFromContext(cmd).Mode != cli.ModeTUI {
					return fmt.Errorf("login requires --no-input in JSON, agent, or non-TTY mode")
				}
				if err := promptAuthLogin(cmd, &profileName, &baseURL, &authType, &email, &username, &backend, &onePasswordAccount, &vault, &item, &credential); err != nil {
					return err
				}
			}
			if jsonInput != "" {
				var input struct {
					ProfileName        string `json:"profile_name"`
					BaseURL            string `json:"base_url"`
					AuthType           string `json:"auth_type"`
					Email              string `json:"email"`
					Username           string `json:"username"`
					Backend            string `json:"backend"`
					OnePasswordAccount string `json:"onepassword_account"`
					Vault              string `json:"vault"`
					Item               string `json:"item"`
				}
				if err := cmdutil.ReadJSONFile(jsonInput, &input); err != nil {
					return err
				}
				if input.ProfileName != "" && !cmd.Flags().Changed("profile-name") {
					profileName = input.ProfileName
				}
				if input.BaseURL != "" && !cmd.Flags().Changed("base-url") {
					baseURL = input.BaseURL
				}
				if input.AuthType != "" && !cmd.Flags().Changed("auth-type") {
					authType = input.AuthType
				}
				if input.Email != "" && !cmd.Flags().Changed("email") {
					email = input.Email
				}
				if input.Username != "" && !cmd.Flags().Changed("username") {
					username = input.Username
				}
				if input.Backend != "" && !cmd.Flags().Changed("backend") {
					backend = input.Backend
				}
				if input.OnePasswordAccount != "" && !cmd.Flags().Changed("onepassword-account") {
					onePasswordAccount = input.OnePasswordAccount
				}
				if input.Vault != "" && !cmd.Flags().Changed("vault") {
					vault = input.Vault
				}
				if input.Item != "" && !cmd.Flags().Changed("item") {
					item = input.Item
				}
			}
			// Under --no-input, refuse to silently fabricate profile
			// metadata. Require baseURL at minimum; profileName
			// defaults are reasonable but baseURL is the load-bearing
			// piece a profile needs.
			if noInput && strings.TrimSpace(baseURL) == "" {
				return fmt.Errorf("validation: --no-input requires --base-url (or --json-input with base_url)")
			}
			if profileName == "" {
				profileName = "default"
			}
			if authType == "" {
				authType = string(config.AuthTypeToken)
			}
			if err := validateAuthLoginType(authType); err != nil {
				return err
			}
			if !authLoginNeedsCredential(authType) {
				credential = ""
				secretStdin = false
				credentialEnv = ""
			}
			targetBackend := config.SecretBackend(backend)
			if err := validateSecretBackend(backend); err != nil {
				return err
			}
			if secretStdin {
				b, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return err
				}
				// Trim only the trailing record delimiter; an explicitly
				// empty credential is rejected, not stored.
				credential, err = config.ReadSecret(string(b))
				if err != nil {
					return fmt.Errorf("validation: --secret-stdin: %w", err)
				}
			}
			if credentialEnv != "" {
				secret, err := config.ReadSecret(os.Getenv(credentialEnv))
				if err != nil {
					return fmt.Errorf("validation: credential environment variable %q: %w", credentialEnv, err)
				}
				credential = secret
			}
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			// : merge with existing profile instead of wholesale replace.
			// Start from the persisted profile (if any) so that fields not
			// supplied to `auth login` (email, account_id, default_project,
			// read_only, editor, …) survive a partial update.
			previousProfile := cmdutil.ExistingProfileOrDefault(cfg, profileName)
			profile := previousProfile
			if cmd.Flags().Changed("profile-name") || profile.Name == "" {
				profile.Name = profileName
			}
			if baseURL != "" {
				profile.BaseURL = config.NormalizeBaseURL(baseURL)
			}
			if cmd.Flags().Changed("auth-type") {
				profile.AuthType = config.AuthType(authType)
			} else if profile.AuthType == "" {
				profile.AuthType = config.AuthType(authType)
			}
			if email != "" {
				profile.Email = email
			}
			if username != "" {
				profile.Username = username
			}
			if cmd.Flags().Changed("backend") {
				profile.SecretBackend = targetBackend
			} else if profile.SecretBackend == "" {
				profile.SecretBackend = targetBackend
			}
			if onePasswordAccount != "" {
				profile.OnePasswordAccount = onePasswordAccount
			}
			if vault != "" {
				profile.Vault = vault
			}
			if item != "" {
				profile.Item = item
			}
			if profile.RefreshInterval == 0 {
				profile.RefreshInterval = config.DefaultRefreshIntervalSeconds
			}
			if profile.TimeoutSeconds == 0 {
				profile.TimeoutSeconds = config.DefaultTimeoutSeconds
			}
			if profile.WorkdaySeconds == 0 {
				profile.WorkdaySeconds = config.DefaultWorkdaySeconds
			}
			cmdutil.UpsertProfile(cfg, profile)
			cfg.DefaultProfile = profileName
			if err := cfg.Validate(); err != nil {
				return err
			}
			// Persist the profile, and — when a credential was supplied — the
			// credential, as one transaction: the credential is staged into
			// the backend, the config is saved, and a save failure rolls the
			// credential write back so a failed login never leaves an orphaned
			// secret in the keyring or 1Password.
			saveConfig := func() error { return config.Save(cmdutil.ConfigPath(cmd), cfg) }
			if credential != "" {
				ref, refErr := cmdutil.SecretRefFor(profile, targetBackend)
				if refErr != nil {
					return refErr
				}
				if err := config.StoreCredentialTransactionally(cmd.Context(), cmdutil.CredentialStoreFor(targetBackend), ref, credential, saveConfig); err != nil {
					return err
				}
			} else if err := saveConfig(); err != nil {
				return err
			}
			// When this login re-pointed an existing profile at a different
			// credential identity (a new site, a backend switch, a different
			// 1Password account/vault/item), the credential under the OLD
			// identity is now stale — revoke it so no live secret lingers in
			// the old keyring entry / old 1Password item. The new credential
			// and config save have already committed, so a cleanup failure is
			// surfaced as a note rather than failing the login.
			if note := revokeOldCredentialOnRelogin(cmd, previousProfile, profile); note != "" {
				cmdutil.RecordCredentialWarnings(cmd, []string{note})
			}
			return cmdutil.WriteEnvelope(cmd, "auth.login", map[string]any{
				"profile":             profileName,
				"auth_type":           authType,
				"secret_backend":      backend,
				"onepassword_account": onePasswordAccount,
				"stored_secret":       credential != "",
			})
		},
	}
	cmd.Flags().StringVar(&profileName, "profile-name", "default", "Profile name to configure")
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Jira base URL")
	cmd.Flags().StringVar(&authType, "auth-type", "token", "Auth type")
	cmd.Flags().StringVar(&email, "email", "", "Jira Cloud account email")
	cmd.Flags().StringVar(&username, "username", "", "Jira Server/Data Center username")
	cmd.Flags().StringVar(&backend, "backend", string(config.SecretBackendKeyring), "Secret backend: keyring or 1password")
	cmd.Flags().StringVar(&onePasswordAccount, "onepassword-account", "", "1Password desktop app account name")
	cmd.Flags().StringVar(&vault, "vault", "", "1Password vault name")
	cmd.Flags().StringVar(&item, "item", "", "1Password item name")
	cmd.Flags().BoolVar(&secretStdin, "secret-stdin", false, "Read credential from stdin")
	cmd.Flags().StringVar(&credentialEnv, "credential-env", "", "Read credential from environment variable")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read auth profile metadata from JSON file")
	// --secret-stdin and --credential-env both supply the credential. Passing
	// both is a syntactic conflict: one would silently win by processing
	// order, so reject it in Cobra validation before any source is read.
	// --json-input carries only profile metadata, never the credential, so it
	// is not part of this group.
	cmd.MarkFlagsMutuallyExclusive("secret-stdin", "credential-env")
	clib.Extend(cmd.Flags().Lookup("profile-name"), clib.FlagExtra{Placeholder: "NAME", Complete: "predictor=profile"})
	clib.Extend(cmd.Flags().Lookup("auth-type"), clib.FlagExtra{Placeholder: "TYPE", Enum: []string{"token", "basic", "pat", "mtls"}, EnumDefault: "token"})
	clib.Extend(cmd.Flags().Lookup("backend"), clib.FlagExtra{Placeholder: "BACKEND", Enum: []string{"keyring", "1password"}, EnumDefault: "keyring"})
	return cmd
}

type authLoginQuestionKind string

const (
	authLoginQuestionInput  authLoginQuestionKind = "input"
	authLoginQuestionSelect authLoginQuestionKind = "select"
)

type authLoginOption struct {
	Label       string
	Value       string
	Description string
}

type authLoginQuestion struct {
	ID          string
	Kind        authLoginQuestionKind
	Title       string
	Description string
	Required    bool
	Secret      bool
	Options     []authLoginOption
}

func authLoginQuestions() []authLoginQuestion {
	return []authLoginQuestion{
		{
			ID:          "profile_name",
			Kind:        authLoginQuestionInput,
			Title:       "Profile name",
			Description: "Short local name for this Jira account, for example work or personal.",
			Required:    true,
		},
		{
			ID:          "base_url",
			Kind:        authLoginQuestionInput,
			Title:       "Jira base URL",
			Description: "Full Jira URL including https scheme, for example https://company.atlassian.net.",
			Required:    true,
		},
		{
			ID:          "auth_type",
			Kind:        authLoginQuestionSelect,
			Title:       "Authentication method",
			Description: "Select the credential type Jira expects for this profile.",
			Required:    true,
			Options: []authLoginOption{
				{Label: "Jira Cloud API token", Value: string(config.AuthTypeToken), Description: "Email plus API token, sent as basic auth."},
				{Label: "Basic auth", Value: string(config.AuthTypeBasic), Description: "Username plus password or token for Server/Data Center."},
				{Label: "Personal access token", Value: string(config.AuthTypePAT), Description: "Bearer-style PAT for Jira Server/Data Center."},
				{Label: "mTLS metadata", Value: string(config.AuthTypeMTLS), Description: "Client certificate/key metadata for Jira Server/Data Center."},
			},
		},
		{
			ID:          "account",
			Kind:        authLoginQuestionInput,
			Title:       "Account email or username",
			Description: "Jira Cloud API tokens use the Atlassian account email; Server/Data Center profiles usually use a username.",
			Required:    true,
		},
		{
			ID:          "secret_backend",
			Kind:        authLoginQuestionSelect,
			Title:       "Secret backend",
			Description: "Choose where jira stores the credential. Config TOML only stores metadata.",
			Required:    true,
			Options: []authLoginOption{
				{Label: "System keychain", Value: string(config.SecretBackendKeyring), Description: "macOS Keychain, Windows Credential Manager, or Linux Secret Service."},
				{Label: "1Password", Value: string(config.SecretBackendOnePassword), Description: "Store the credential in a 1Password vault using the Go SDK when configured."},
			},
		},
		{
			ID:          "credential",
			Kind:        authLoginQuestionInput,
			Title:       "Credential",
			Description: "API token, PAT, or password to store in the selected secret backend.",
			Required:    true,
			Secret:      true,
		},
		{
			ID:          "onepassword_account",
			Kind:        authLoginQuestionInput,
			Title:       "1Password account",
			Description: "Desktop app account name for SDK auth. Leave blank to use OP_SERVICE_ACCOUNT_TOKEN.",
		},
		{
			ID:          "vault",
			Kind:        authLoginQuestionInput,
			Title:       "1Password vault",
			Description: "Vault name used when the 1Password backend is selected.",
		},
		{
			ID:          "item",
			Kind:        authLoginQuestionInput,
			Title:       "1Password item",
			Description: "Item title for this Jira profile in the selected 1Password vault.",
		},
	}
}

func promptAuthLogin(cmd *cobra.Command, profileName, baseURL, authType, email, username, backend, onePasswordAccount, vault, item, credential *string) error {
	account := cmdutil.FirstNonEmpty(*email, *username)
	form := authLoginForm(profileName, baseURL, authType, &account, backend, onePasswordAccount, vault, item, credential).
		WithInput(cmd.InOrStdin()).
		WithOutput(cmd.ErrOrStderr())
	if err := form.RunWithContext(cmd.Context()); err != nil {
		// Return a typed PromptError so MapError classifies the outcome
		// via errors.As — never let "aborted"/"canceled" reach the
		// substring classifier, where it would be misread as an auth
		// failure.
		switch {
		case errors.Is(err, huh.ErrUserAborted):
			return cli.NewPromptError(cli.PromptAborted, "auth login", err)
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return cli.NewPromptError(cli.PromptCanceled, "auth login", err)
		default:
			return err
		}
	}
	switch config.AuthType(*authType) {
	case config.AuthTypeBasic, config.AuthTypePAT, config.AuthTypeMTLS:
		*username = strings.TrimSpace(account)
		*email = ""
	default:
		*email = strings.TrimSpace(account)
		*username = ""
	}
	trimAuthLoginValues(profileName, baseURL, authType, email, username, backend, onePasswordAccount, vault, item, credential)
	return nil
}

func authLoginForm(profileName, baseURL, authType, account, backend, onePasswordAccount, vault, item, credential *string) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Profile name").
				Description("Short local name for this Jira account, for example work or personal.").
				Value(profileName).
				Validate(requiredString("profile name is required")),
			huh.NewInput().
				Title("Jira base URL").
				Description("Full URL or shorthand. \"company\" expands to https://company.atlassian.net; full URLs (https://...) are accepted as-is.").
				Value(baseURL).
				Validate(func(value string) error {
					value = strings.TrimSpace(value)
					if value == "" {
						return errors.New("jira base URL is required")
					}
					return config.ValidateBaseURL(config.NormalizeBaseURL(value))
				}),
			huh.NewSelect[string]().
				Title("Authentication method").
				Description("Select the credential type Jira expects for this profile.").
				Options(authLoginHuhOptions(authLoginQuestionByID(authLoginQuestions(), "auth_type").Options)...).
				Value(authType).
				Validate(validateAuthLoginType),
			huh.NewInput().
				Title("Account email or username").
				Description("Jira Cloud API tokens use the Atlassian account email; Server/Data Center profiles usually use a username.").
				Value(account).
				Validate(func(value string) error {
					switch config.AuthType(*authType) {
					case config.AuthTypeToken, config.AuthTypeBasic:
						if strings.TrimSpace(value) == "" {
							return errors.New("account email or username is required")
						}
					}
					return nil
				}),
		).Title("Jira profile").Description("Configure the Jira instance and account identity."),
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Secret backend").
				Description("Choose where jira stores the credential. Config TOML only stores metadata.").
				Options(authLoginHuhOptions(authLoginQuestionByID(authLoginQuestions(), "secret_backend").Options)...).
				Value(backend).
				Validate(validateSecretBackend),
			huh.NewInput().
				Title("Credential").
				Description("API token, PAT, or password to store in the selected secret backend.").
				EchoMode(huh.EchoModePassword).
				Value(credential).
				Validate(requiredString("credential is required")),
		).Title("Credential storage").Description("Secrets are written to the selected backend, not to config.toml.").WithHideFunc(func() bool {
			return !authLoginNeedsCredential(*authType)
		}),
		huh.NewGroup(
			huh.NewInput().
				Title("1Password account").
				Description("Desktop app account name for SDK auth. Leave blank to use OP_SERVICE_ACCOUNT_TOKEN.").
				Value(onePasswordAccount),
			huh.NewInput().
				Title("1Password vault").
				Description("Vault name used when the 1Password backend is selected.").
				Value(vault).
				Validate(func(value string) error {
					if config.SecretBackend(*backend) == config.SecretBackendOnePassword && strings.TrimSpace(value) == "" {
						return errors.New("1Password vault is required")
					}
					return nil
				}),
			huh.NewInput().
				Title("1Password item").
				Description("Item title for this Jira profile in the selected 1Password vault.").
				Value(item).
				Validate(func(value string) error {
					if config.SecretBackend(*backend) == config.SecretBackendOnePassword && strings.TrimSpace(value) == "" {
						return errors.New("1Password item is required")
					}
					return nil
				}),
		).Title("1Password").Description("Only used when the 1Password backend is selected.").WithHideFunc(func() bool {
			return config.SecretBackend(*backend) != config.SecretBackendOnePassword
		}),
	)
}

func authLoginQuestionByID(questions []authLoginQuestion, id string) authLoginQuestion {
	for _, question := range questions {
		if question.ID == id {
			return question
		}
	}
	return authLoginQuestion{}
}

func authLoginHuhOptions(options []authLoginOption) []huh.Option[string] {
	out := make([]huh.Option[string], 0, len(options))
	for _, option := range options {
		out = append(out, huh.NewOption(option.Label+" - "+option.Description, option.Value))
	}
	return out
}

func requiredString(message string) func(string) error {
	return func(value string) error {
		if strings.TrimSpace(value) == "" {
			return errors.New(message)
		}
		return nil
	}
}

func validateAuthLoginType(value string) error {
	switch config.AuthType(value) {
	case config.AuthTypeToken, config.AuthTypeBasic, config.AuthTypePAT, config.AuthTypeMTLS:
		return nil
	default:
		return fmt.Errorf("unsupported auth type %q", value)
	}
}

func authLoginNeedsCredential(authType string) bool {
	switch config.AuthType(authType) {
	case config.AuthTypeToken, config.AuthTypeBasic, config.AuthTypePAT:
		return true
	default:
		return false
	}
}

func validateSecretBackend(value string) error {
	switch config.SecretBackend(value) {
	case config.SecretBackendKeyring, config.SecretBackendOnePassword:
		return nil
	default:
		return fmt.Errorf("unsupported secret backend %q", value)
	}
}

func trimAuthLoginValues(values ...*string) {
	for _, value := range values {
		*value = strings.TrimSpace(*value)
	}
}

func authStatusCommand() *cobra.Command {
	var noProbe bool
	var projectKey string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show authentication status",
		Long: `Reports credential resolution and (by default) probes the live Jira API
to surface how the token actually behaves end-to-end. Pass --no-probe
to skip remote calls and run only the local credential check.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profiles := make([]map[string]any, 0, len(cfg.Profiles))
			for _, profile := range cfg.Profiles {
				entry := map[string]any{"profile": profile.Name}
				ref, refErr := cmdutil.SecretRefFor(profile, profile.SecretBackend)
				if refErr != nil {
					entry["valid"] = false
					entry["error"] = refErr.Error()
					profiles = append(profiles, entry)
					continue
				}
				cred := config.CredentialStatus(
					cmd.Context(),
					cmdutil.CredentialStoreFor(profile.SecretBackend),
					ref,
				)
				entry["valid"] = cred.Valid
				entry["source"] = cred.Source
				entry["redacted"] = cred.Redacted
				if cred.Error != "" {
					entry["error"] = cred.Error
				}
				if !noProbe && cred.Valid && profile.BaseURL != "" {
					entry["remote"] = probeRemoteAuth(cmd, profile, projectKey)
				}
				profiles = append(profiles, entry)
			}
			return cmdutil.WriteEnvelope(cmd, "auth.status", map[string]any{
				"active_profile": cfg.DefaultProfile,
				"profiles":       profiles,
			})
		},
	}
	cmd.Flags().BoolVar(&noProbe, "no-probe", false, "Skip the remote /myself + /mypermissions check")
	cmd.Flags().StringVar(&projectKey, "project", "", "Probe permissions in the context of this project")
	clib.Extend(cmd.Flags().Lookup("project"), clib.FlagExtra{Placeholder: "KEY", Complete: "predictor=cacheproject"})
	return cmd
}

// probeRemoteAuth makes lightweight HTTP calls to surface how the token
// actually behaves, distinguishing "credential resolves locally" from
// "credential is accepted by Jira and grants the permissions the CLI
// needs". Returns a map suitable for inclusion in the auth.status
// envelope. Never errors — every failure becomes a structured field so
// the user can see what went wrong without rerunning anything.
func probeRemoteAuth(cmd *cobra.Command, profile config.Profile, projectKey string) map[string]any {
	out := map[string]any{"site": profile.BaseURL}

	// Route the probe through the same client constructor normal commands
	// use, so the per-profile request timeout and an mTLS client certificate
	// both apply to the probe's live calls rather than silently defaulting.
	client, _, ok, err := cmdutil.JiraClientForProfile(cmd, profile)
	if err != nil {
		out["error"] = config.SanitizeCredentialError(err)
		return out
	}
	if !ok || client == nil {
		out["error"] = "profile has no base URL to probe"
		return out
	}
	user := jira.NewUserService(client)

	// /myself reveals scope-level auth issues (granular tokens missing
	// read:user/group/avatar/application-role 4-scope union).
	myselfOut := map[string]any{}
	if me, resp, err := user.Myself(cmd.Context()); err != nil {
		myselfOut["ok"] = false
		myselfOut["status"] = httpStatusOf(resp)
		myselfOut["error"] = err.Error()
		if httpStatusOf(resp) == 401 {
			myselfOut["hint"] = "token can't read /myself — for granular tokens, add read:user:jira, read:application-role:jira, read:group:jira, read:avatar:jira (Atlassian requires the union)"
		}
	} else {
		myselfOut["ok"] = true
		myselfOut["account_id"] = me.AccountID
		myselfOut["email"] = me.EmailAddress
	}
	out["myself"] = myselfOut

	// /mypermissions surfaces resource-level access (the "scoped token
	// without resource grants" failure mode).
	keys := []string{
		"BROWSE_PROJECTS", "CREATE_ISSUES", "EDIT_ISSUES", "DELETE_ISSUES",
		"ADD_COMMENTS", "WORK_ON_ISSUES", "TRANSITION_ISSUES", "LINK_ISSUES",
	}
	permsOut := map[string]any{}
	if pr, resp, err := user.MyPermissions(cmd.Context(), projectKey, keys); err != nil {
		permsOut["ok"] = false
		permsOut["status"] = httpStatusOf(resp)
		permsOut["error"] = err.Error()
	} else {
		permsOut["ok"] = true
		grants := map[string]bool{}
		anyGranted := false
		for _, k := range keys {
			have := pr.Permissions[k].HavePermission
			grants[k] = have
			if have {
				anyGranted = true
			}
		}
		permsOut["grants"] = grants
		if !anyGranted {
			permsOut["hint"] = "token authenticates but has zero CLI-relevant permissions — check the Resources tab of your scoped token at id.atlassian.com (KAN/SAM1 etc. must be selected)"
		}
	}
	out["permissions"] = permsOut

	return out
}

func httpStatusOf(resp *jira.Response) int {
	if resp == nil || resp.Response == nil {
		return 0
	}
	return resp.Response.StatusCode
}

func authLogoutCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "logout PROFILE",
		Short:             "Remove stored credentials",
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='profile'"},
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(args[0])
			if err != nil {
				return err
			}
			ref, err := cmdutil.SecretRefFor(profile, profile.SecretBackend)
			if err != nil {
				return err
			}
			// Revoke the credential from its backend. An absent credential is
			// not an error: removed=false reports there was nothing to remove.
			// A note is returned when revocation left a user-named 1Password
			// item in place; surface it as an informational warning.
			removed, note, err := config.RevokeProfileCredential(cmd.Context(), cmdutil.CredentialStoreFor(profile.SecretBackend), ref)
			if err != nil {
				return err
			}
			if note != "" {
				cmdutil.RecordCredentialWarnings(cmd, []string{note})
			}
			return cmdutil.WriteEnvelope(cmd, "auth.logout", map[string]any{"profile": profile.Name, "removed": removed})
		},
	}
}

func authSwitchCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "switch PROFILE",
		Short:             "Switch active profile",
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='profile'"},
		ValidArgsFunction: completeProfileNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(args[0])
			if err != nil {
				return err
			}
			cfg.DefaultProfile = profile.Name
			if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
				return err
			}
			return cmdutil.WriteEnvelope(cmd, "auth.switch", map[string]any{"active": profile.Name})
		},
	}
}

func authRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
			if err != nil {
				return err
			}
			data := map[string]any{
				"profile":   profile.Name,
				"auth_type": string(profile.AuthType),
				"refreshed": false,
			}
			data["reason"] = "selected auth type has no refresh flow"
			return cmdutil.WriteEnvelope(cmd, "auth.refresh", data)
		},
	}
}

func authMigrateCommand() *cobra.Command {
	var backend, onePasswordAccount, vault, item string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate credentials between backends",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := config.SecretBackend(backend)
			if target != config.SecretBackendKeyring && target != config.SecretBackendOnePassword {
				return fmt.Errorf("unsupported secret backend %q", backend)
			}
			cfg, err := config.LoadOrInit(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			// When --profile is explicitly set, resolve it so a typo is
			// rejected here rather than silently matching no profile and
			// returning success with an empty result.
			profileName := cmdutil.RequestedProfile(cmd)
			if profileName != "" {
				resolved, rerr := cfg.ResolveProfile(profileName)
				if rerr != nil {
					return rerr
				}
				profileName = resolved.Name
			}
			// --item names ONE 1Password item; applying it across a whole
			// batch would point every profile.s credential at a single
			// vault item. Require a single selected profile when --item is set.
			if item != "" && profileName == "" && len(cfg.Profiles) > 1 {
				return fmt.Errorf("--item names a single 1Password item; use --profile to select one profile, or omit --item to let each profile use its own item")
			}
			ops := make([]map[string]any, 0, len(cfg.Profiles))
			// migrations collects the profiles that need a real backend
			// switch. The destination write for every one of them is staged
			// before the single config.Save, so a save failure rolls the
			// whole batch back rather than stranding new secrets. Each
			// CredentialMigration carries its profile index, so migrations is
			// the single ordered source of truth for which profiles flip.
			// migratingOps holds the matching op result entries, appended in
			// lockstep with migrations so the two never drift.
			var migrations []config.CredentialMigration
			var migratingOps []map[string]any
			for i := range cfg.Profiles {
				profile := &cfg.Profiles[i]
				if profileName != "" && profile.Name != profileName {
					continue
				}
				op := map[string]any{
					"profile":        profile.Name,
					"source_backend": string(profile.SecretBackend),
					"target_backend": string(target),
					"dry_run":        dryRun,
					"migrated":       false,
				}
				if profile.SecretBackend == target {
					op["reason"] = "already using target backend"
					ops = append(ops, op)
					continue
				}
				if target == config.SecretBackendOnePassword {
					if onePasswordAccount != "" {
						profile.OnePasswordAccount = onePasswordAccount
					}
					if vault != "" {
						profile.Vault = vault
					}
					if item != "" {
						profile.Item = item
					}
					// Leave profile.Item empty when the user did not name an
					// item: CredentialIdentity derives the default name and
					// marks the resulting 1Password item jira-cli-owned, so a
					// later migrate-away can safely delete it. Persisting the
					// default name would make jira-cli's own item look
					// user-named and strand it on the next migration.
					if profile.Vault == "" {
						op["error"] = "1Password migration requires --vault or existing profile vault metadata"
						ops = append(ops, op)
						continue
					}
				}
				if dryRun {
					op["reason"] = "dry run"
					ops = append(ops, op)
					continue
				}
				sourceRef, refErr := cmdutil.SecretRefFor(*profile, profile.SecretBackend)
				if refErr != nil {
					op["error"] = refErr.Error()
					ops = append(ops, op)
					continue
				}
				destRef, refErr := cmdutil.SecretRefFor(*profile, target)
				if refErr != nil {
					op["error"] = refErr.Error()
					ops = append(ops, op)
					continue
				}
				migrations = append(migrations, config.CredentialMigration{
					Profile:      profile.Name,
					ProfileIndex: i,
					Source:       cmdutil.CredentialStoreFor(profile.SecretBackend),
					Destination:  cmdutil.CredentialStoreFor(target),
					SourceRef:    sourceRef,
					DestRef:      destRef,
				})
				migratingOps = append(migratingOps, op)
				ops = append(ops, op)
			}
			cleanupFailures := []string{}
			cleanupNotes := []string{}
			if !dryRun && len(migrations) > 0 {
				report, migErr := config.MigrateCredentials(cmd.Context(), migrations, func() error {
					// The destination secrets are staged; persist the new
					// backend metadata. Only a durable save here lets the
					// source secrets be cleaned up.
					applyMigratedBackends(cfg, migrations, target)
					return config.Save(cmdutil.ConfigPath(cmd), cfg)
				})
				if migErr != nil {
					return fmt.Errorf("auth migrate: %w", migErr)
				}
				for _, op := range migratingOps {
					op["migrated"] = true
				}
				for _, failure := range report.CleanupFailures {
					cleanupFailures = append(cleanupFailures, failure.Profile)
				}
				// A cleanup note marks source storage left in place because
				// jira-cli does not own it. Surface it to the user so they
				// can remove the old credential by hand if they want to.
				for _, note := range report.CleanupNotes {
					cleanupNotes = append(cleanupNotes, note.Message)
				}
				cmdutil.RecordCredentialWarnings(cmd, cleanupNotes)
			} else if !dryRun {
				if err := config.Save(cmdutil.ConfigPath(cmd), cfg); err != nil {
					return err
				}
			}
			data := map[string]any{
				"target_backend": string(target),
				"dry_run":        dryRun,
				"profiles":       ops,
			}
			if len(cleanupFailures) > 0 {
				data["cleanup_failures"] = cleanupFailures
			}
			if len(cleanupNotes) > 0 {
				data["cleanup_notes"] = cleanupNotes
			}
			return cmdutil.WriteEnvelope(cmd, "auth.migrate", data)
		},
	}
	cmd.Flags().StringVar(&backend, "backend", string(config.SecretBackendKeyring), "Target secret backend: keyring or 1password")
	cmd.Flags().StringVar(&onePasswordAccount, "onepassword-account", "", "1Password desktop app account name")
	cmd.Flags().StringVar(&vault, "vault", "", "1Password vault name")
	cmd.Flags().StringVar(&item, "item", "", "1Password item name")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview credential migration")
	clib.Extend(cmd.Flags().Lookup("backend"), clib.FlagExtra{Placeholder: "BACKEND", Enum: []string{"keyring", "1password"}, EnumDefault: "keyring"})
	return cmd
}

func authTokenCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "token",
		Short: "Show redacted token diagnostics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(cmdutil.ConfigPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(cmdutil.RequestedProfile(cmd))
			if err != nil {
				return err
			}
			ref, refErr := cmdutil.SecretRefFor(profile, profile.SecretBackend)
			if refErr != nil {
				return refErr
			}
			status := config.CredentialStatus(cmd.Context(), cmdutil.CredentialStoreFor(profile.SecretBackend), ref)
			data := map[string]any{
				"profile":  profile.Name,
				"source":   status.Source,
				"backend":  string(profile.SecretBackend),
				"valid":    status.Valid,
				"redacted": status.Redacted,
				"expiry":   nil,
				"error":    status.Error,
			}
			// 1Password coordinates (account/vault/item) are meaningful
			// only for a 1Password-backed profile. Populating them on a
			// keyring profile prints fields irrelevant to the active
			// backend, so scope them to the 1Password backend.
			if profile.SecretBackend == config.SecretBackendOnePassword {
				data["onepassword_account"] = profile.OnePasswordAccount
				data["vault"] = profile.Vault
				data["item"] = cmdutil.FirstNonEmpty(profile.Item, "jira-cli-"+profile.Name)
			}
			return cmdutil.WriteEnvelope(cmd, "auth.token", data)
		},
	}
}

// profileBaseURLEnvVar returns the name of the environment variable that
// overrides a profile's base_url. It mirrors the JIRA_PROFILE_<NAME>_*
// convention parsed by the config loader: the profile name is uppercased
// with '-' replaced by '_'.
func profileBaseURLEnvVar(name string) string {
	return "JIRA_PROFILE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_BASE_URL"
}

// revokeOldCredentialOnRelogin revokes the previous credential after an auth
// login re-points an existing profile at a different credential identity (a
// new site, a backend switch, a different 1Password account/vault/item). The
// old credential would otherwise linger as a live secret in the old keyring
// entry / old 1Password item. It is called only after the new credential
// write and config save have committed.
//
// It returns an informational note when cleanup of the old credential failed;
// an empty string means there was nothing to revoke (fresh profile, unchanged
// identity) or the revocation succeeded. Cleanup failure is surfaced, never
// fatal — the login itself already succeeded.
func revokeOldCredentialOnRelogin(cmd *cobra.Command, previous, updated config.Profile) string {
	// A profile that was never persisted (no prior base_url) has no old
	// credential to revoke.
	if previous.BaseURL == "" {
		return ""
	}
	oldRef, err := cmdutil.SecretRefFor(previous, previous.SecretBackend)
	if err != nil {
		return ""
	}
	newRef, err := cmdutil.SecretRefFor(updated, updated.SecretBackend)
	if err != nil {
		return ""
	}
	return revokeOldCredential(cmd.Context(), cmdutil.CredentialStoreFor(previous.SecretBackend), oldRef, newRef)
}

// revokeOldCredential revokes the credential at oldRef when it addresses
// storage different from newRef. For the keyring backend the old entry is
// deleted; for 1Password the old item's managed credential field is stripped
// (the item itself is never destroyed). When the identities are the same it
// does nothing — the credential was not re-pointed. A cleanup failure is
// returned as an informational note, not an error: the login already
// succeeded and must not be undone by a failed best-effort cleanup.
func revokeOldCredential(ctx context.Context, store config.CredentialStore, oldRef, newRef config.SecretRef) string {
	if !config.CredentialIdentitiesDiffer(oldRef, newRef) {
		return ""
	}
	if _, _, err := config.RevokeProfileCredential(ctx, store, oldRef); err != nil {
		return fmt.Sprintf(
			"the previous credential for profile %q (%s, site %s) could not be removed: %v — remove it manually",
			oldRef.Profile, oldRef.Backend, oldRef.Host, err,
		)
	}
	return ""
}

// applyMigratedBackends flips secret_backend to target on exactly the
// profiles named by migrations, located by the ProfileIndex each migration
// carries. Profiles not in the batch are left untouched.
func applyMigratedBackends(cfg *config.Config, migrations []config.CredentialMigration, target config.SecretBackend) {
	for _, m := range migrations {
		if m.ProfileIndex >= 0 && m.ProfileIndex < len(cfg.Profiles) {
			cfg.Profiles[m.ProfileIndex].SecretBackend = target
		}
	}
}
