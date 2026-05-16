package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/huh"
	clib "github.com/gechr/clib/cli/cobra"
	"github.com/gechr/x/terminal"
	"github.com/matcra587/jira-cli/internal/cli"
	"github.com/matcra587/jira-cli/internal/cli/adfmode"
	stdininput "github.com/matcra587/jira-cli/internal/cli/stdin"
	"github.com/matcra587/jira-cli/internal/config"
	editorpkg "github.com/matcra587/jira-cli/internal/editor"
	"github.com/matcra587/jira-cli/internal/pipeline"
	"github.com/matcra587/jira-cli/internal/version"
	"github.com/matcra587/jira-cli/pkg/adf"
	"github.com/matcra587/jira-cli/pkg/jira"
	"github.com/spf13/cobra"
)

func registerCommands(root *cobra.Command) {
	root.AddCommand(
		tuiCommand(),
		schemaCommand(), // top-level alias preserved for backward compatibility
		agentCommand(),
		cacheCommand(),
		meCommand(),
		versionCommand(),
		authCommand(),
		issueCommand(),
		boardsCommand(),
		epicCommand(),
		jqlCommand(),
		aliasCommand(),
		searchCommand(),
		worklogCommand(),
		configCommand(),
	)
}

func versionCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "version",
		Short:   "Print version information",
		GroupID: "agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeEnvelope(cmd, "version", map[string]any{
				"version":    version.Version,
				"commit":     version.Commit,
				"branch":     version.Branch,
				"build_time": version.BuildTime,
				"build_by":   version.BuildBy,
				"summary":    version.String(),
			})
		},
	}
}

func tuiCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Short:   "Launch the persistent dashboard",
		GroupID: "dashboard",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := cli.RequireTTY(os.Stdout)
			if err != nil {
				return err
			}
			_, err = tuiRun(cmd)
			return err
		},
	}
}

func schemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "schema",
		Short:   "Print command and JSON schema metadata",
		GroupID: "agent",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return writeSchema(cmd)
		},
	}
}

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
				cfg, loadErr = config.LoadOrInit(config.WithPath(configPath(cmd)))
				if loadErr != nil {
					return loadErr
				}
				resolved, resolveErr := cfg.ResolveProfile(requestedProfile(cmd))
				if resolveErr != nil {
					if errors.Is(resolveErr, config.ErrProfileNotDefined) {
						return fmt.Errorf("validation: cannot --save profile %q: it is not defined in the config file (it exists only via a JIRA_* env overlay)", requestedProfile(cmd))
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
				client, profile, ok, clientErr = jiraClientForProfile(cmd, resolved)
			} else {
				client, profile, ok, clientErr = jiraClientForCommand(cmd)
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
				if err := config.Save(configPath(cmd), cfg); err != nil {
					return err
				}
				data["saved"] = true
			}
			return writeEnvelope(cmd, "auth.whoami", data)
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
				if DetectorFromContext(cmd).Mode != cli.ModePlain && DetectorFromContext(cmd).Mode != cli.ModeTUI {
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
				if err := readJSONFile(jsonInput, &input); err != nil {
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
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			// : merge with existing profile instead of wholesale replace.
			// Start from the persisted profile (if any) so that fields not
			// supplied to `auth login` (email, account_id, default_project,
			// read_only, editor, …) survive a partial update.
			previousProfile := existingProfileOrDefault(cfg, profileName)
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
			upsertProfile(cfg, profile)
			cfg.DefaultProfile = profileName
			if err := cfg.Validate(); err != nil {
				return err
			}
			// Persist the profile, and — when a credential was supplied — the
			// credential, as one transaction: the credential is staged into
			// the backend, the config is saved, and a save failure rolls the
			// credential write back so a failed login never leaves an orphaned
			// secret in the keyring or 1Password.
			saveConfig := func() error { return config.Save(configPath(cmd), cfg) }
			if credential != "" {
				ref, refErr := secretRefFor(profile, targetBackend)
				if refErr != nil {
					return refErr
				}
				if err := config.StoreCredentialTransactionally(cmd.Context(), credentialStoreFor(targetBackend), ref, credential, saveConfig); err != nil {
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
				recordCredentialWarnings(cmd, []string{note})
			}
			return writeEnvelope(cmd, "auth.login", map[string]any{
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
	account := firstNonEmpty(*email, *username)
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
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			profiles := make([]map[string]any, 0, len(cfg.Profiles))
			for _, profile := range cfg.Profiles {
				entry := map[string]any{"profile": profile.Name}
				ref, refErr := secretRefFor(profile, profile.SecretBackend)
				if refErr != nil {
					entry["valid"] = false
					entry["error"] = refErr.Error()
					profiles = append(profiles, entry)
					continue
				}
				cred := config.CredentialStatus(
					cmd.Context(),
					credentialStoreFor(profile.SecretBackend),
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
			return writeEnvelope(cmd, "auth.status", map[string]any{
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
	client, _, ok, err := jiraClientForProfile(cmd, profile)
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
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(args[0])
			if err != nil {
				return err
			}
			ref, err := secretRefFor(profile, profile.SecretBackend)
			if err != nil {
				return err
			}
			// Revoke the credential from its backend. An absent credential is
			// not an error: removed=false reports there was nothing to remove.
			// A note is returned when revocation left a user-named 1Password
			// item in place; surface it as an informational warning.
			removed, note, err := config.RevokeProfileCredential(cmd.Context(), credentialStoreFor(profile.SecretBackend), ref)
			if err != nil {
				return err
			}
			if note != "" {
				recordCredentialWarnings(cmd, []string{note})
			}
			return writeEnvelope(cmd, "auth.logout", map[string]any{"profile": profile.Name, "removed": removed})
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
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(args[0])
			if err != nil {
				return err
			}
			cfg.DefaultProfile = profile.Name
			if err := config.Save(configPath(cmd), cfg); err != nil {
				return err
			}
			return writeEnvelope(cmd, "auth.switch", map[string]any{"active": profile.Name})
		},
	}
}

func authRefreshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "Refresh credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(requestedProfile(cmd))
			if err != nil {
				return err
			}
			data := map[string]any{
				"profile":   profile.Name,
				"auth_type": string(profile.AuthType),
				"refreshed": false,
			}
			data["reason"] = "selected auth type has no refresh flow"
			return writeEnvelope(cmd, "auth.refresh", data)
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
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			// When --profile is explicitly set, resolve it so a typo is
			// rejected here rather than silently matching no profile and
			// returning success with an empty result.
			profileName := requestedProfile(cmd)
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
				sourceRef, refErr := secretRefFor(*profile, profile.SecretBackend)
				if refErr != nil {
					op["error"] = refErr.Error()
					ops = append(ops, op)
					continue
				}
				destRef, refErr := secretRefFor(*profile, target)
				if refErr != nil {
					op["error"] = refErr.Error()
					ops = append(ops, op)
					continue
				}
				migrations = append(migrations, config.CredentialMigration{
					Profile:      profile.Name,
					ProfileIndex: i,
					Source:       credentialStoreFor(profile.SecretBackend),
					Destination:  credentialStoreFor(target),
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
					return config.Save(configPath(cmd), cfg)
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
				recordCredentialWarnings(cmd, cleanupNotes)
			} else if !dryRun {
				if err := config.Save(configPath(cmd), cfg); err != nil {
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
			return writeEnvelope(cmd, "auth.migrate", data)
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
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			profile, err := cfg.ResolveProfile(requestedProfile(cmd))
			if err != nil {
				return err
			}
			ref, refErr := secretRefFor(profile, profile.SecretBackend)
			if refErr != nil {
				return refErr
			}
			status := config.CredentialStatus(cmd.Context(), credentialStoreFor(profile.SecretBackend), ref)
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
				data["item"] = firstNonEmpty(profile.Item, "jira-cli-"+profile.Name)
			}
			return writeEnvelope(cmd, "auth.token", data)
		},
	}
}

func issueCommand() *cobra.Command {
	cmd := groupCommand("issue", "Work with Jira issues", "resources")
	cmd.AddCommand(issueListCommand())
	cmd.AddCommand(issueMineCommand())
	cmd.AddCommand(issueViewCommand())
	cmd.AddCommand(issueCreateCommand())
	cmd.AddCommand(issueEditCommand())
	cmd.AddCommand(issueTransitionCommand())
	cmd.AddCommand(issueCommentGroup())
	cmd.AddCommand(IssueAttachmentCommand())
	cmd.AddCommand(issueLinkSubCommand())
	cmd.AddCommand(issueWebLinkCommand())
	for _, mk := range WatcherCommands {
		cmd.AddCommand(mk())
	}
	cmd.AddCommand(destructiveIssueCommand("clone", "Clone an issue"))
	cmd.AddCommand(destructiveIssueCommand("move", "Move an issue"))
	cmd.AddCommand(destructiveIssueCommand("delete", "Delete an issue"))
	return cmd
}

// issueMineCommand is a thin shorthand for `jira issue list --assignee me`.
// Shares the same runner as `issue list` so any future change to the list
// path (caching, output shape, …) propagates without diverging.
func issueMineCommand() *cobra.Command {
	var opts issueListOptions
	cmd := &cobra.Command{
		Use:   "mine",
		Short: `List issues assigned to you (alias for "issue list --assignee me")`,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts.builder.Assignee = "me"
			return runIssueList(cmd, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.detail, "detail", false, "Fetch full issue records")
	cmd.Flags().StringVar(&opts.jqlQuery, "jql", "", "Add custom JQL clauses (combined with assignee = currentUser())")
	cmd.Flags().BoolVar(&opts.asJQL, "as-jql", false, "Print the built JQL without calling Jira")
	cmd.Flags().StringSliceVar(&opts.builder.Statuses, "status", nil, "Restrict by status name")
	cmd.Flags().StringSliceVar(&opts.builder.Projects, "project", nil, "Restrict by project key")
	return cmd
}

func issueViewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "view KEY",
		Short: "View issue details",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				issue, resp, err := issueService(client).Get(cmd.Context(), args[0], &jira.IssueGetOptions{Expand: []string{"renderedFields", "names", "schema", "transitions", "operations"}})
				if err != nil {
					return err
				}
				// ADF render-loss warnings describe what the HUMAN
				// renderer drops when it flattens ADF to Markdown. The
				// json/compact envelope carries the full ADF in
				// data.issue, so nothing is lost there — emitting the
				// warning on a machine path would be false.
				// Scope the scan to the human output mode only.
				var warnings []adf.Warning
				if usePlainOutput(cmd) {
					warnings = collectIssueLossyWarnings(issue)
				}
				return writeEnvelopeWithResponseAndWarnings(cmd, "issue.view", map[string]any{"issue": issue}, resp, warnings)
			}
			return writeEnvelope(cmd, "issue.view", map[string]any{"issue": map[string]any{"key": args[0]}})
		},
	}
}

// collectIssueLossyWarnings inspects an *jira.Issue for ADF surfaces
// (description, embedded comment bodies) and returns a structured
// warning per surface that contained at least one construct the
// ADF→Markdown renderer can't fully express. The warning shape mirrors
// the comment-list lossy-warning contract: Field identifies the source
// (`description` or `comment:<id>`), NodeType is the dropped node
// name, and Message enumerates the construct list. Multiple lossy
// constructs on a single source produce multiple warnings (one per
// construct) so they fit the existing flat cli.Warning envelope shape;
// consumers that want a per-source rollup can group on Field.
func collectIssueLossyWarnings(issue *jira.Issue) []adf.Warning {
	if issue == nil {
		return nil
	}
	var warnings []adf.Warning
	if issue.Fields != nil && issue.Fields.Description != nil {
		res := adf.ToMarkdownLossy(*issue.Fields.Description)
		for _, c := range res.LossyConstructs {
			warnings = append(warnings, adf.Warning{
				Type:     "adf_lossy_render",
				Message:  fmt.Sprintf("description ADF construct %q dropped during Markdown render; render in --output=json for full ADF fidelity", c),
				Field:    "description",
				NodeType: c,
				Lossy:    true,
			})
		}
	}
	for _, comment := range issue.Comments {
		if comment == nil || comment.Body == nil {
			continue
		}
		res := adf.ToMarkdownLossy(*comment.Body)
		if len(res.LossyConstructs) == 0 {
			continue
		}
		commentID := ""
		if comment.ID != nil {
			commentID = *comment.ID
		}
		field := "comment"
		if commentID != "" {
			field = "comment:" + commentID
		}
		for _, c := range res.LossyConstructs {
			warnings = append(warnings, adf.Warning{
				Type:     "adf_lossy_render",
				Message:  fmt.Sprintf("comment %s ADF construct %q dropped during Markdown render; render in --output=json for full ADF fidelity", commentID, c),
				Field:    field,
				NodeType: c,
				Lossy:    true,
			})
		}
	}
	return warnings
}

// issueListOptions captures every input the issue-list runner needs.
// Both `issue list` and `issue mine` populate it from their flags.
type issueListOptions struct {
	builder  jqlBuildOptions
	jqlQuery string
	detail   bool
	asJQL    bool
}

// runIssueList is the shared body for `issue list` and `issue mine`. It
// applies profile defaults, builds the JQL, optionally short-circuits with
// --as-jql, then either calls Jira or returns an empty envelope when no
// client is configured. Output flows through the same `issue.list` envelope
// shape so consumers can't tell which command emitted it.
func runIssueList(cmd *cobra.Command, opts issueListOptions) error {
	scope, precedence, scopeErr := boardScopeFromFlags(cmd)
	if scopeErr != nil {
		return scopeErr
	}
	//  default_board wins exclusively over default_project on
	// commands that consume --board. When the board scope is active,
	// the builder must NOT inherit default_project — the board's
	// project keys are the sole project clause.
	scopeActive := len(scope.Board.ProjectKeys) > 0
	if opts.asJQL {
		// --as-jql must not require a credential — it never calls Jira.
		// boardScopeFromFlags is cache-only (no client probe), and we
		// load the profile directly here instead of jiraClientForCommand
		// so the secret backend (e.g. 1Password) stays untouched.
		cfg, cfgErr := config.Load(config.WithPath(configPath(cmd)))
		if cfgErr != nil {
			return cfgErr
		}
		profile := activeProfile(cmd, cfg)
		builder := opts.builder
		if !scopeActive {
			builder = issueListBuilderWithProfileDefaults(builder, profile)
		}
		query, err := issueListJQL(opts.jqlQuery, builder)
		if err != nil {
			return err
		}
		query = applyBoardClauseToJQL(query, scope)
		return writeEnvelope(cmd, "issue.list.jql", boardScopedListData(cmd, []map[string]any{}, opts.detail, query, scope, precedence))
	}
	client, profile, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	builder := opts.builder
	if !scopeActive {
		builder = issueListBuilderWithProfileDefaults(builder, profile)
	}
	query, err := issueListJQL(opts.jqlQuery, builder)
	if err != nil {
		return err
	}
	query = applyBoardClauseToJQL(query, scope)
	if !ok {
		return writeEnvelope(cmd, "issue.list", boardScopedListData(cmd, []map[string]any{}, opts.detail, query, scope, precedence))
	}
	service := issueService(client)
	issues, resp, err := service.List(cmd.Context(), &jira.IssueListOptions{
		ListOptions: jira.ListOptions{MaxResults: 50},
		JQL:         query,
	})
	if err != nil {
		return err
	}
	if opts.detail {
		issues, err = fetchIssueDetails(cmd.Context(), service, issues)
		if err != nil {
			return err
		}
	}
	issueData := issueOutput(issues, opts.detail)
	return writeEnvelopeWithResponse(cmd, "issue.list", boardScopedListData(cmd, issueData, opts.detail, query, scope, precedence), resp)
}

// applyBoardClauseToJQL prepends the board's `project in (...)` clause
// onto an existing JQL string with top-level AND. Returns the input
// unchanged when scope contributes no clause (no-flag-no-default).
func applyBoardClauseToJQL(query string, scope jira.BoardScope) string {
	clause, ok := scope.JQLClause()
	if !ok {
		return query
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return clause
	}
	// Insert the clause before any ORDER BY suffix so the resulting
	// expression remains a valid `<filters> ORDER BY <field>` query.
	upper := strings.ToUpper(q)
	if idx := strings.Index(upper, " ORDER BY "); idx != -1 {
		return clause + " AND " + q[:idx] + q[idx:]
	}
	return clause + " AND " + q
}

// boardScopedListData extends issueListOutputData with the new envelope
// fields per contracts/envelope-shapes.md > issue list --board.
func boardScopedListData(cmd *cobra.Command, issues any, detail bool, query string, scope jira.BoardScope, precedence string) map[string]any {
	data := issueListOutputData(cmd, issues, detail, query)
	data["jql"] = query
	data["precedence"] = precedence
	data["board_scope"] = boardScopeEnvelopeData(scope)
	return data
}

// issueListCommand has been relocated to cmd/jira/issue_list.go per the
// `issue_<verb>.go` convention. The runner (runIssueList) and helpers
// (issueListBuilderWithProfileDefaults, issueListOutputData) remain in
// this file because they are shared with `issue mine`.

func issueListBuilderWithProfileDefaults(builder jqlBuildOptions, profile config.Profile) jqlBuildOptions {
	if len(compactStrings(builder.Projects)) == 0 && profile.DefaultProject != "" {
		builder.Projects = []string{profile.DefaultProject}
	}
	return builder
}

func issueListOutputData(cmd *cobra.Command, issues any, detail bool, query string) map[string]any {
	data := map[string]any{"issues": issues, "detail": detail}
	if debug, _ := cmd.Root().PersistentFlags().GetBool("debug"); debug {
		data["jql"] = query
	}
	return data
}

func fetchIssueDetails(ctx context.Context, service jira.IssueService, summaries []*jira.Issue) ([]*jira.Issue, error) {
	details := make([]*jira.Issue, 0, len(summaries))
	for _, issue := range summaries {
		key := ""
		if issue != nil && issue.Key != nil {
			key = *issue.Key
		}
		if key == "" {
			details = append(details, issue)
			continue
		}
		detail, _, err := service.Get(ctx, key, &jira.IssueGetOptions{Expand: []string{"renderedFields", "names", "schema", "transitions", "operations", "changelog"}})
		if err != nil {
			return nil, err
		}
		details = append(details, detail)
	}
	return details, nil
}

func issueCreateCommand() *cobra.Command {
	var dryRun bool
	var summary, jsonInput, assignee string
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an issue",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			noInput := noInputRequested(cmd)
			payload := map[string]any{"summary": summary}
			if jsonInput != "" {
				if err := readJSONFile(jsonInput, &payload); err != nil {
					return err
				}
			}
			// Resolve the profile WITHOUT building a client: --assignee me,
			// --no-input validation, and the dry-run preview only need
			// profile metadata. Credentials are resolved later, at the
			// live-submit boundary.
			profile, err := profileForCommand(cmd)
			if err != nil {
				return err
			}
			// --assignee shortcut: feeds the spec's assignee_account_id input
			// when "me" / a literal account-id is supplied. "none" clears it.
			if v := strings.TrimSpace(assignee); v != "" {
				switch strings.ToLower(v) {
				case "none", "unassigned":
					delete(payload, "assignee_account_id")
				case "me", "@me":
					if profile.AccountID != "" {
						payload["assignee_account_id"] = profile.AccountID
					}
				default:
					payload["assignee_account_id"] = v
				}
			}
			if noInput {
				if err := validateIssueCreateRequired(payload, profile); err != nil {
					return err
				}
			}
			// The issue description is the primary ADF document for
			// this mutation. Whether it arrived as `description_markdown`
			// or as a raw ADF `description`, it is pulled out of the
			// payload here and fed to the pipeline as ADFDoc so stage 2
			// (ValidateDoc + ApplyCompatibility) runs on it BEFORE
			// submission — never as a post-pipeline conversion that
			// skips validation. The post-pipeline SubmitADF is the only
			// description that reaches the wire.
			descriptionDoc, descriptionPresent, descMarkdownWarnings, descErr := extractDescriptionDoc(payload)
			if descErr != nil {
				return descErr
			}
			// Extract any remaining ADF-shaped subfields (e.g.
			// `environment`, `customfield_NNNN` ADF) so stage 2 validates
			// them. `description` was already removed above, so it is
			// not double-validated here. These named docs are
			// validate-only: with no per-field screen schema we cannot
			// know their compatibility envelope, so ApplyCompatibility is
			// not run on them — the same treatment the primary
			// description receives (see extractDescriptionDoc).
			namedADF, adfParseErr := extractNamedADFDocs(payload)
			if adfParseErr != nil {
				return adfParseErr
			}

			// Route through the 5-stage validation pipeline before
			// submission. Without an active screen schema, stages
			// 1+2+4 still run; stage 3 is a no-op (no schema means
			// no "off-screen" check possible).
			pipeIn := pipeline.MutationInput{
				Mode:             adfModeFor(cmd, true),
				Fields:           payload,
				DryRun:           dryRun,
				NamedADFDocs:     namedADF,
				MarkdownWarnings: descMarkdownWarnings,
			}
			if descriptionPresent {
				// FieldCompatibility is left at its zero value with
				// InlineCardSupported=true: a Jira Cloud `description`
				// field accepts inlineCard, so compatibility degradation
				// would be wrong here. ApplyCompatibility still walks the
				// doc; it just has nothing to degrade.
				pipeIn.ADFDoc = &descriptionDoc
				pipeIn.FieldCompat = &adf.FieldCompatibility{Field: "description", InlineCardSupported: true}
			}
			pipeOut := pipeline.RunMutation(pipeIn)
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Every path past validation uses the pipeline's
			// post-validation, post-encoding output — never the raw
			// pre-pipeline payload. SubmitFields is the only map allowed
			// downstream.
			submitFields := pipeOut.SubmitFields
			if submitFields == nil {
				submitFields = map[string]any{}
			}
			// The validated, compatibility-applied description from the
			// pipeline replaces whatever the payload carried — markdown
			// and raw ADF are now handled identically.
			if descriptionPresent && pipeOut.SubmitADF != nil {
				submitFields["description"] = *pipeOut.SubmitADF
			}
			if dryRun {
				preview, err := issueCreatePreview(submitFields, profile)
				if err != nil {
					return err
				}
				return writeEnvelopeWithWarnings(cmd, "issue.create", map[string]any{
					"preview": preview,
					"dry_run": true,
				}, pipeOut.Warnings)
			}
			client, profile, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.create")
			}
			req := &jira.IssueCreateRequest{
				Summary:   stringFromAny(submitFields["summary"]),
				Project:   firstNonEmpty(stringFromAny(submitFields["project_key"]), profile.DefaultProject),
				IssueType: firstNonEmpty(stringFromAny(submitFields["issue_type"]), profile.DefaultIssueType),
				Fields:    submitFields,
			}
			issue, resp, err := issueService(client).Create(cmd.Context(), req)
			if err != nil {
				return err
			}
			return writeEnvelopeWithResponseAndWarnings(cmd, "issue.create", map[string]any{"issue": issue, "dry_run": false}, resp, pipeOut.Warnings)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmd.Flags().StringVar(&summary, "summary", "", "Issue summary")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read issue create payload from JSON file")
	cmd.Flags().StringVar(&assignee, "assignee", "", `Assign on creation: "me" or a Jira account ID`)
	return cmd
}

// configuredEditorFor returns the editor.Resolve(...) "configured"
// argument for the active invocation: the active profile's Editor
// field if set, otherwise the global Config.Editor. The resolver in
// internal/editor layers $JIRA_EDITOR / $EDITOR / $VISUAL / "vi" on
// top of whatever this returns.
func configuredEditorFor(cmd *cobra.Command) string {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return ""
	}
	name := profileForEnvelope(cmd)
	if name != "" {
		profile := cfg.Profile(name)
		if v := strings.TrimSpace(profile.Editor); v != "" {
			return v
		}
	}
	return strings.TrimSpace(cfg.Editor)
}

// adfModeFor resolves the ADF strict/best-effort mode for a single
// mutation invocation. mutation=true selects the mutation-submit
// default (strict) when nothing else overrides.
func adfModeFor(cmd *cobra.Command, mutation bool) adfmode.Mode {
	flag := adfmode.FlagUnset
	if v, _ := cmd.Flags().GetBool("adf-strict"); v {
		flag |= adfmode.FlagStrict
	}
	if v, _ := cmd.Flags().GetBool("adf-best-effort"); v {
		flag |= adfmode.FlagBestEffort
	}
	env := os.Getenv("JIRA_ADF_STRICT")
	// TODO: thread profile.ADFStrict once a typed *bool field lands on
	// internal/config.Profile (currently only env + flag + default-by-path).
	var profilePtr *bool
	path := adfmode.PathRead
	if mutation {
		path = adfmode.PathMutationSubmit
	}
	mode, err := adfmode.Resolve(adfmode.Inputs{Flag: flag, Env: env, Profile: profilePtr, Path: path})
	if err != nil {
		// Resolver only errors on conflicting inputs; default to safe.
		if mutation {
			return adfmode.ModeStrict
		}
		return adfmode.ModeBestEffort
	}
	return mode
}

// readOnlyEnabled reports whether the active profile (or the JIRA_READ_ONLY
// env var) blocks mutations. Env wins on the OFF→ON direction so an agent
// shell can enforce read-only globally without editing config.
func readOnlyEnabled(cmd *cobra.Command) bool {
	if envReadOnlyEnabled() {
		return true
	}
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		// On config-load failure, fail safe: treat as writable so the
		// real command surfaces the underlying error rather than masking
		// it with a read-only refusal.
		return false
	}
	return activeProfile(cmd, cfg).ReadOnly
}

// dryRunRequested reports whether the active command was invoked with
// --dry-run. The flag is declared per-command (not persistent), so the
// lookup tolerates its absence: commands without a --dry-run flag
// simply return false. Threading this into the Jira client lets the
// service layer refuse any mutating request as a dry-run safety net.
func dryRunRequested(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		// Command has no --dry-run flag — nothing to honor.
		return false
	}
	v, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		// The flag exists but is not a bool (a future redefinition).
		// Fail SAFE: a dry-run guard that cannot read its flag must
		// assume dry-run is ON rather than silently disabling itself.
		return true
	}
	return v
}

func envReadOnlyEnabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("JIRA_READ_ONLY")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// resolveAssigneeField turns an --assignee flag value into the Jira `assignee`
// field shape. Accepted values:
//   - ""                         → no change (set ok=false)
//   - "me" / "@me"               → profile.AccountID (Cloud) or profile.Username (Server/DC)
//   - "none" / "unassigned"      → nil (clears assignee)
//   - "<accountId>"              → {accountId: "..."}
//
// Returns (value, set, error). set=false with err=nil means no flag was given
// (no field change). err is non-nil when input was supplied but couldn't be
// resolved — the caller MUST surface this rather than silently dropping it,
// because Jira Cloud ignores email-based assignment without complaint.
func resolveAssigneeField(input string, profile config.Profile) (any, bool, error) {
	v := strings.TrimSpace(input)
	if v == "" {
		return nil, false, nil
	}
	switch strings.ToLower(v) {
	case "none", "unassigned":
		return nil, true, nil
	case "me", "@me":
		switch {
		case profile.AccountID != "":
			return map[string]string{"accountId": profile.AccountID}, true, nil
		case profile.Username != "":
			return map[string]string{"name": profile.Username}, true, nil
		}
		return nil, false, fmt.Errorf("--assignee me requires profile.account_id (Cloud) or profile.username (Server/DC); run `jira auth whoami --save` to populate it")
	default:
		return map[string]string{"accountId": v}, true, nil
	}
}

// validateIssueCreateRequired enforces the spec rule "headless write commands
// require complete input via --no-input + --json-input". It checks that
// project_key, issue_type, and summary are derivable from the supplied JSON
// payload or profile defaults, otherwise returns a validation error.
func validateIssueCreateRequired(payload map[string]any, profile config.Profile) error {
	var missing []string
	if firstNonEmpty(stringFromAny(payload["summary"])) == "" {
		missing = append(missing, "summary")
	}
	if firstNonEmpty(stringFromAny(payload["project_key"]), profile.DefaultProject) == "" {
		missing = append(missing, "project_key")
	}
	if firstNonEmpty(stringFromAny(payload["issue_type"]), profile.DefaultIssueType) == "" {
		missing = append(missing, "issue_type")
	}
	if len(missing) > 0 {
		return fmt.Errorf("--no-input requires complete input: missing required fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

// issueCreatePreview builds the dry-run preview shape per command-schemas.md.
// The supplied fields map is the pipeline's post-validation SubmitFields:
// the description (whether it arrived as markdown or raw ADF) has already
// been converted, validated, and compatibility-applied, and lands here
// under the bare `description` key. Profile defaults fill missing
// project_key / issue_type when the JSON payload omits them.
func issueCreatePreview(fields map[string]any, profile config.Profile) (map[string]any, error) {
	preview := map[string]any{
		"project_key": firstNonEmpty(stringFromAny(fields["project_key"]), profile.DefaultProject),
		"issue_type":  firstNonEmpty(stringFromAny(fields["issue_type"]), profile.DefaultIssueType),
		"summary":     stringFromAny(fields["summary"]),
	}
	// description in SubmitFields is the validated ADF document. Surface
	// it under description_adf so the preview names the wire shape.
	if doc, ok := fields["description"]; ok && doc != nil {
		preview["description_adf"] = doc
	}
	for _, key := range []string{"assignee_account_id", "priority", "labels", "components", "epic_key", "custom_fields"} {
		if v, ok := fields[key]; ok {
			preview[key] = v
		}
	}
	return preview, nil
}

// extractDescriptionDoc pulls the issue description out of an issue-create
// payload as a single adf.Document, removing the source key(s) from the
// map so the description is processed exactly once — as the pipeline's
// primary ADFDoc, not as an opaque named subfield.
//
// Two input shapes are accepted, in priority order:
//   - `description_markdown`: a Markdown string, converted via
//     FromMarkdownLossy. Conversion warnings are returned so the pipeline
//     can abort (strict) or surface them (best-effort) on content loss.
//   - `description`: a raw ADF document object.
//
// present is false when neither key is set. When present, the returned
// doc is what the pipeline validates; the caller writes the pipeline's
// SubmitADF back into the fields map under `description`.
func extractDescriptionDoc(payload map[string]any) (doc adf.Document, present bool, warnings []adf.Warning, err error) {
	if md := stringFromAny(payload["description_markdown"]); md != "" {
		delete(payload, "description_markdown")
		delete(payload, "description")
		converted, convWarnings, cerr := adf.FromMarkdownLossy(md)
		if cerr != nil {
			return adf.Document{}, false, nil, fmt.Errorf("convert description_markdown to ADF: %w", cerr)
		}
		return converted, true, convWarnings, nil
	}
	raw, ok := payload["description"]
	if !ok || raw == nil {
		return adf.Document{}, false, nil, nil
	}
	delete(payload, "description")
	encoded, merr := json.Marshal(raw)
	if merr != nil {
		return adf.Document{}, false, nil, fmt.Errorf("marshal description for ADF validation: %w", merr)
	}
	parsed, _, perr := adf.Parse(encoded)
	if perr != nil {
		return adf.Document{}, false, nil, fmt.Errorf("description: %w", perr)
	}
	return parsed, true, nil, nil
}

func issueEditCommand() *cobra.Command {
	var dryRun bool
	var jsonInput, summary, assignee string
	cmd := &cobra.Command{
		Use:   "edit KEY",
		Short: "Edit an issue",
		Long: `Edit a Jira issue.

With no field flags, opens the configured external editor on the issue
description (kubectl-style). Use --summary / --assignee / --json-input
for headless or single-field edits.

In headless mode (--no-input), at least one field flag MUST be provided
— there is no editor to open and silent no-ops are validation errors.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := noInputRequested(cmd)
			payload := map[string]any{"fields": map[string]any{}}
			if jsonInput != "" {
				if err := readJSONFile(jsonInput, &payload); err != nil {
					return err
				}
			}
			fields, ok := payload["fields"].(map[string]any)
			if !ok {
				return fmt.Errorf("issue edit JSON input must contain a fields object")
			}
			// --summary / --assignee shortcuts, applied on top of any --json-input.
			// Resolve the profile only — building a client here would
			// resolve credentials even on a dry-run or editor-only path.
			profile, err := profileForCommand(cmd)
			if err != nil {
				return err
			}
			if v := strings.TrimSpace(summary); v != "" {
				fields["summary"] = v
			}
			if v, set, err := resolveAssigneeField(assignee, profile); err != nil {
				return err
			} else if set {
				fields["assignee"] = v
			}
			// kubectl-style default: bare `jira issue edit KEY` (no field
			// flags, no --json-input) opens the configured external editor
			// on the issue description. The editor reads keystrokes from
			// stdin, so the gate checks stdin specifically — piping stdout
			// (`jira issue edit KEY | tee out.json`) is a legitimate human
			// workflow that must NOT trip the refusal. det.Agent covers
			// LLM-agent harnesses regardless of stdin shape.
			if len(fields) == 0 {
				if noInput {
					return fmt.Errorf("validation: no fields specified for issue edit; provide --summary, --assignee, or --json-input")
				}
				det := DetectorFromContext(cmd)
				if det.Agent || !stdininput.IsTerminal() {
					return fmt.Errorf("validation: issue edit requires an interactive terminal for the editor flow; in agent or non-TTY context, provide --summary, --assignee, or --json-input")
				}
				return issueEditWithEditor(cmd, args[0], dryRun)
			}
			// Any ADF-shaped value in the fields object (e.g. a raw
			// `description` document supplied via --json-input) MUST be
			// validated by stage 2 before submission — otherwise garbage
			// nested ADF would only be checked structurally by the
			// customfield encoder, which does not enforce ADF rules.
			namedADF, adfParseErr := extractNamedADFDocs(fields)
			if adfParseErr != nil {
				return adfParseErr
			}
			// Thread the mutation through the 5-stage pipeline.
			pipeOut := pipeline.RunMutation(pipeline.MutationInput{
				Mode:         adfModeFor(cmd, true),
				Fields:       fields,
				NamedADFDocs: namedADF,
				DryRun:       dryRun,
			})
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Submit and preview the validated SubmitFields, not
			// the raw pre-pipeline fields map.
			submitFields := pipeOut.SubmitFields
			if submitFields == nil {
				submitFields = map[string]any{}
			}
			if !dryRun {
				client, _, hasClient, err := jiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if !hasClient {
					return fmt.Errorf("jira base URL is required for issue.edit")
				}
				issue, resp, err := issueService(client).Update(cmd.Context(), args[0], &jira.IssueUpdateRequest{Fields: submitFields})
				if err != nil {
					return err
				}
				return writeEnvelopeWithResponseAndWarnings(cmd, "issue.edit", map[string]any{
					"issue":   args[0],
					"result":  issue,
					"dry_run": false,
					"fields":  submitFields,
				}, resp, pipeOut.Warnings)
			}
			return writeEnvelopeWithWarnings(cmd, "issue.edit", map[string]any{
				"issue":   args[0],
				"dry_run": true,
				"fields":  submitFields,
			}, pipeOut.Warnings)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read issue edit payload from JSON file")
	cmd.Flags().StringVar(&summary, "summary", "", "Replace the issue summary")
	cmd.Flags().StringVar(&assignee, "assignee", "", `Set assignee: "me", "none"/"unassigned", or a Jira account ID`)
	return cmd
}

func issueEditWithEditor(cmd *cobra.Command, key string, dryRun bool) error {
	client, _, ok, err := jiraClientForCommand(cmd)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("jira base URL is required for issue.edit --edit")
	}
	issueService := issueService(client)
	issue, _, err := issueService.Get(cmd.Context(), key, &jira.IssueGetOptions{})
	if err != nil {
		return err
	}
	doc := adf.Document{Type: "doc", Version: 1}
	if issue != nil && issue.Fields != nil && issue.Fields.Description != nil {
		doc = *issue.Fields.Description
	}
	// Route the edit through the opaque-preserving round-trip. Blocks
	// with no faithful Markdown representation — panels, tables,
	// inlineCards, mentions — are carried through the editor buffer as
	// protected opaque fences and reconstituted byte-for-byte, so a
	// no-op save can no longer erase rich Jira content. A plain
	// adf.ToMarkdown render would have dropped them before the user
	// ever saw the buffer.
	updatedDoc, editWarnings, err := editorpkg.RoundTripADF(cmd.Context(), editorpkg.RoundTripADFOptions{
		IssueKey:  key,
		FieldName: "description",
		Document:  doc,
		EditCmd:   configuredEditorFor(cmd),
	})
	if err != nil {
		return err
	}
	// External-editor edits ARE mutations and MUST run through the
	// pipeline. The edited description is the only field, and it is an
	// ADF document — route it solely as ADFDoc so stage 2 (ValidateDoc
	// + ApplyCompatibility) owns it. It is deliberately NOT also placed
	// in Fields: a single field must travel one pipeline channel, so
	// there is no last-write-wins reconciliation between SubmitFields
	// and SubmitADF. Stage 4 (customfield encoding) has nothing to do
	// for an empty Fields map.
	//
	// The round-trip's warnings — lossy Markdown conversions on the
	// edited text plus opaque-preservation notices — travel as
	// MarkdownWarnings so strict mode aborts on genuine content loss
	// before submission.
	pipeOut := pipeline.RunMutation(pipeline.MutationInput{
		Mode:             adfModeFor(cmd, true),
		ADFDoc:           &updatedDoc,
		FieldCompat:      &adf.FieldCompatibility{Field: "description", InlineCardSupported: true},
		MarkdownWarnings: editWarnings,
		DryRun:           dryRun,
	})
	if pipeOut.Aborted {
		return pipeOut.Err
	}
	// Submit the validated, compatibility-applied SubmitADF — not
	// the pre-pipeline edit. description is the sole field on the wire.
	submitFields := map[string]any{}
	if pipeOut.SubmitADF != nil {
		submitFields["description"] = *pipeOut.SubmitADF
	}
	if dryRun {
		return writeEnvelopeWithWarnings(cmd, "issue.edit", map[string]any{
			"issue":   key,
			"dry_run": true,
			"fields":  submitFields,
		}, pipeOut.Warnings)
	}
	updatedIssue, resp, err := issueService.Update(cmd.Context(), key, &jira.IssueUpdateRequest{Fields: submitFields})
	if err != nil {
		return err
	}
	return writeEnvelopeWithResponseAndWarnings(cmd, "issue.edit", map[string]any{
		"issue":   key,
		"result":  updatedIssue,
		"dry_run": false,
		"fields":  submitFields,
	}, resp, pipeOut.Warnings)
}

func issueTransitionCommand() *cobra.Command {
	var dryRun bool
	var transitionID string
	returnCmd := &cobra.Command{
		Use:   "transition KEY",
		Short: "Transition an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// A workflow transition is a Jira mutation and MUST run
			// through the 5-stage pipeline. No fields/ADF doc to
			// validate today (transitions don't carry payload here),
			// but the parse + dry-run gating still apply.
			pipeOut := pipeline.RunMutation(pipeline.MutationInput{
				Mode:   adfModeFor(cmd, true),
				DryRun: dryRun,
			})
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			if dryRun {
				return writeEnvelopeWithWarnings(cmd, "issue.transition", map[string]any{"issue": args[0], "transition": transitionID, "dry_run": dryRun}, pipeOut.Warnings)
			}
			if transitionID == "" {
				// List available transitions — this is a READ, not a
				// mutation; it returns successor IDs the caller chooses
				// from. Skip the warnings helper since pipeline only
				// runs to satisfy stage gating consistency.
				client, _, ok, err := jiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					transitions, resp, err := issueService(client).Transitions(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					return writeEnvelopeWithResponseAndWarnings(cmd, "issue.transitions", map[string]any{"issue": args[0], "transitions": transitions}, resp, pipeOut.Warnings)
				}
			}
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok && transitionID != "" {
				resp, err := issueService(client).Transition(cmd.Context(), args[0], &jira.TransitionRequest{ID: transitionID})
				if err != nil {
					return err
				}
				return writeEnvelopeWithResponseAndWarnings(cmd, "issue.transition", map[string]any{"issue": args[0], "transition": transitionID, "dry_run": false}, resp, pipeOut.Warnings)
			}
			if !dryRun && transitionID != "" {
				return fmt.Errorf("jira base URL is required for issue.transition")
			}
			return writeEnvelopeWithWarnings(cmd, "issue.transition", map[string]any{"issue": args[0], "dry_run": dryRun}, pipeOut.Warnings)
		},
	}
	returnCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	returnCmd.Flags().StringVar(&transitionID, "transition", "", "Transition ID to execute")
	return returnCmd
}

func destructiveIssueCommand(name, short string) *cobra.Command {
	var dryRun, force, deleteSubtasks bool
	var jsonInput string
	cmd := &cobra.Command{
		Use:   name + " KEY",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			noInput := noInputRequested(cmd)
			payload := map[string]any{"fields": map[string]any{}}
			if jsonInput != "" {
				if err := readJSONFile(jsonInput, &payload); err != nil {
					return err
				}
			}
			// Destructive commands ARE mutations and MUST run through
			// the pipeline. delete has no fields to validate (stages
			// 2-4 no-op); clone/move carry a fields payload that stage
			// 4 (customfield encoding) rejects on malformed values.
			pipeFields := issueFieldsFromPayload(payload)
			pipeOut := pipeline.RunMutation(pipeline.MutationInput{
				Mode:   adfModeFor(cmd, true),
				Fields: pipeFields,
				DryRun: dryRun,
			})
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Clone/move submit the validated SubmitFields. delete
			// carries no field payload, so SubmitFields is an empty map.
			submitFields := pipeOut.SubmitFields
			if submitFields == nil {
				submitFields = map[string]any{}
			}
			if dryRun {
				return writeEnvelopeWithWarnings(cmd, "issue."+name, map[string]any{"issue": args[0], "payload": map[string]any{"fields": submitFields}, "dry_run": true}, pipeOut.Warnings)
			}
			// Destructive op safety: in TTY mode (a human at the
			// keyboard) require either --force OR an interactive
			// "are you sure?" confirmation. Headless / agent shells
			// MUST pass --force explicitly — the auto-detect refuses
			// to prompt them.
			det := DetectorFromContext(cmd)
			if !force {
				// Non-TTY / agent / --no-input → MUST pass --force.
				// We refuse to prompt headless callers and refuse to
				// proceed without explicit consent.
				if !det.IsTTY || det.Agent || noInput {
					return fmt.Errorf("issue %s requires --force in headless / agent / --no-input mode", name)
				}
				// TTY human → huh confirmation prompt.
				if ok, err := confirmDestructive(cmd, name, args[0]); err != nil {
					return err
				} else if !ok {
					return cli.NewPromptError(cli.PromptAborted, "issue "+name, nil)
				}
			}
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok && !dryRun {
				service := issueService(client)
				var resp *jira.Response
				var issue *jira.Issue
				switch name {
				case "delete":
					resp, err = service.Delete(cmd.Context(), args[0], &jira.IssueDeleteOptions{DeleteSubtasks: deleteSubtasks})
				case "clone":
					issue, resp, err = service.Clone(cmd.Context(), args[0], &jira.IssueCloneRequest{Fields: submitFields})
				case "move":
					issue, resp, err = service.Move(cmd.Context(), args[0], &jira.IssueMoveRequest{Fields: submitFields})
				}
				if err != nil {
					return err
				}
				return writeEnvelopeWithResponseAndWarnings(cmd, "issue."+name, map[string]any{"issue": args[0], "result": issue, "dry_run": false}, resp, pipeOut.Warnings)
			}
			return fmt.Errorf("jira base URL is required for issue.%s", name)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	cmd.Flags().BoolVar(&force, "force", false, "Confirm destructive mutation")
	cmd.Flags().BoolVar(&deleteSubtasks, "delete-subtasks", false, "(delete only) also delete the issue's subtasks (Jira refuses delete otherwise when subtasks exist)")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read mutation payload from JSON file")
	return cmd
}

// confirmDestructive prompts the user via huh for an interactive
// yes/no confirmation before executing a destructive op
// (delete/clone/move). Only invoked in TTY mode; non-TTY callers
// MUST pass --force and are rejected by the caller before reaching
// this function under stdin discipline.
//
// The prompt runs under the command context so a SIGINT or an elapsed
// --timeout cancels the prompt instead of leaving it blocked on the
// terminal. A canceled prompt returns a typed *cli.PromptError so the
// envelope keeps the cancellation identity; a declined confirmation
// returns (false, nil) and the caller turns it into an abort.
func confirmDestructive(cmd *cobra.Command, action, key string) (bool, error) {
	confirmed := false
	confirm := huh.NewConfirm().
		Title(fmt.Sprintf("About to %s %s", action, key)).
		Description("This is destructive. Continue?").
		Affirmative("Yes, " + action).
		Negative("Cancel").
		Value(&confirmed)
	form := huh.NewForm(huh.NewGroup(confirm))
	if err := form.RunWithContext(cmd.Context()); err != nil {
		switch {
		case errors.Is(err, huh.ErrUserAborted):
			// Esc / Ctrl-C inside the form is a decline, not a fault.
			return false, nil
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			return false, cli.NewPromptError(cli.PromptCanceled, action+" confirmation", err)
		default:
			return false, cli.NewPromptError(cli.PromptUnavailable, action+" confirmation", err)
		}
	}
	return confirmed, nil
}

// issueWebLinkCommand wires `jira issue weblink KEY --url URL --title T`.
// Goes through POST /rest/api/3/issue/{key}/remotelink — Jira's
// "Web links" feature, separate from issue-to-issue links.
func issueWebLinkCommand() *cobra.Command {
	var url, title string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "weblink KEY",
		Short: "Attach a web link (URL + title) to an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if url == "" {
				return fmt.Errorf("validation: --url is required")
			}
			// Local URL syntax validation runs in BOTH dry-run and live
			// mode. dry-run is a local preview: it checks the URL parses
			// and carries an absolute http/https scheme, but it cannot
			// and does not verify the target is reachable.
			if err := validateWebLinkURL(url); err != nil {
				return err
			}
			if dryRun {
				return writeEnvelope(cmd, "issue.weblink", map[string]any{
					"issue": args[0], "url": url, "title": title, "dry_run": true,
					// Be explicit that dry-run did NOT contact the
					// target URL — only its syntax was checked locally.
					"url_remote_checked": false,
				})
			}
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("jira base URL is required for issue.weblink")
			}
			resp, err := issueService(client).AddRemoteLink(cmd.Context(), args[0], &jira.RemoteLinkRequest{
				URL: url, Title: title,
			})
			if err != nil {
				return err
			}
			return writeEnvelopeWithResponse(cmd, "issue.weblink", map[string]any{
				"issue": args[0], "url": url, "title": title, "dry_run": false,
			}, resp)
		},
	}
	cmd.Flags().StringVar(&url, "url", "", "Web link target URL (required)")
	cmd.Flags().StringVar(&title, "title", "", "Display title for the link")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview without creating the link")
	return cmd
}

// validateWebLinkURL performs the local, offline syntax check the
// weblink dry-run promises: the value must parse and carry an absolute
// http/https URL. It deliberately does NOT fetch the target — dry-run
// is local preview only, and reachability is not its job.
func validateWebLinkURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("validation: --url %q is not a valid URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("validation: --url %q must be an absolute http or https URL", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("validation: --url %q is missing a host", raw)
	}
	return nil
}

func issueFieldsFromPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	if rawFields, ok := payload["fields"]; ok {
		if fields, ok := rawFields.(map[string]any); ok {
			return copyAnyMap(fields)
		}
	}
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		if key == "dry_run" {
			continue
		}
		out[key] = value
	}
	return out
}

func copyAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	maps.Copy(out, in)
	return out
}

func worklogAddCommand() *cobra.Command {
	var timeSpent, commentMarkdown, started, jsonInput string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add KEY",
		Short: "Add a worklog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// commentADF carries a canonical ADF `comment` document
			// supplied via --json-input. It is mutually exclusive with
			// the Markdown comment form.
			var commentADF *adf.Document
			if jsonInput != "" {
				var input struct {
					TimeSpent       string          `json:"time_spent"`
					TimeSpentLegacy string          `json:"timeSpent"`
					Started         string          `json:"started"`
					CommentMarkdown string          `json:"comment_markdown"`
					Comment         json.RawMessage `json:"comment"`
				}
				if err := readJSONFile(jsonInput, &input); err != nil {
					return err
				}
				if input.TimeSpent == "" {
					input.TimeSpent = input.TimeSpentLegacy
				}
				if input.TimeSpent != "" && !cmd.Flags().Changed("time-spent") {
					timeSpent = input.TimeSpent
				}
				if input.Started != "" && !cmd.Flags().Changed("started") {
					started = input.Started
				}
				if input.CommentMarkdown != "" && !cmd.Flags().Changed("comment-markdown") {
					commentMarkdown = input.CommentMarkdown
				}
				// `comment` is the canonical ADF document shape — the
				// same shape `issue comment --json-input` accepts. It is
				// parsed and validated through the pipeline below.
				if len(input.Comment) > 0 && string(input.Comment) != "null" {
					if input.CommentMarkdown != "" {
						return fmt.Errorf("validation: worklog input has both 'comment' (ADF) and 'comment_markdown'; provide exactly one")
					}
					parsed, _, perr := adf.Parse(input.Comment)
					if perr != nil {
						return fmt.Errorf("worklog --json-input comment: %w", perr)
					}
					commentADF = &parsed
				}
			}
			seconds, err := jira.ParseDuration(timeSpent, workdaySecondsForCommand(cmd))
			if err != nil {
				return err
			}
			var comment *adf.Document
			var commentMarkdownWarnings []adf.Warning
			switch {
			case commentADF != nil:
				comment = commentADF
			case commentMarkdown != "":
				doc, convWarnings, err := adf.FromMarkdownLossy(commentMarkdown)
				if err != nil {
					return err
				}
				comment = &doc
				commentMarkdownWarnings = convWarnings
			}
			// Thread worklog comment ADF through the pipeline.
			pipeOut := pipeline.RunMutation(pipeline.MutationInput{
				Mode:             adfModeFor(cmd, true),
				ADFDoc:           comment,
				MarkdownWarnings: commentMarkdownWarnings,
				DryRun:           dryRun,
			})
			if pipeOut.Aborted {
				return pipeOut.Err
			}
			// Submit the validated SubmitADF (post-compatibility),
			// not the pre-pipeline comment doc.
			comment = pipeOut.SubmitADF
			if !dryRun {
				client, _, ok, err := jiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					worklog, resp, err := worklogService(client).Add(cmd.Context(), args[0], &jira.WorklogAddRequest{TimeSpentSeconds: seconds, Started: started, Comment: comment})
					if err != nil {
						return err
					}
					return writeEnvelopeWithResponseAndWarnings(cmd, "worklog.add", map[string]any{"issue": args[0], "worklog": worklog, "dry_run": false}, resp, pipeOut.Warnings)
				}
				return fmt.Errorf("jira base URL is required for worklog.add")
			}
			return writeEnvelopeWithWarnings(cmd, "worklog.add", map[string]any{
				"issue": args[0],
				"worklog": map[string]any{
					"time_spent_seconds": seconds,
					"started":            started,
					"comment":            comment,
				},
				"dry_run": dryRun,
			}, pipeOut.Warnings)
		},
	}
	cmd.Flags().StringVar(&timeSpent, "time-spent", "", "Human-readable time spent")
	cmd.Flags().StringVar(&started, "started", "", "Worklog start timestamp")
	cmd.Flags().StringVar(&commentMarkdown, "comment-markdown", "", "Worklog comment as Markdown")
	cmd.Flags().StringVar(&jsonInput, "json-input", "", "Read worklog payload from JSON file")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	return cmd
}

func writeEnvelope(cmd *cobra.Command, command string, data any) error {
	return writeEnvelopeWithWarnings(cmd, command, data, nil)
}

// writeEnvelopeWithRawWarnings emits the standard envelope shape with a
// caller-supplied list of free-form warning maps. Necessary for warnings
// whose schema (cache-truncated, rate-limit-during-paginate) carries
// fields outside the cli.Warning struct's Type/Message/Field/Path/etc.
// surface — see contracts/envelope-shapes.md.
func writeEnvelopeWithRawWarnings(cmd *cobra.Command, command string, data any, warnings []map[string]any) error {
	for _, cw := range collectedCredentialWarnings(cmd) {
		warnings = append(warnings, map[string]any{
			"type":    cw.Type,
			"message": cw.Message,
			"lossy":   cw.Lossy,
		})
	}
	if useCompactOutput(cmd) {
		// compact is the data payload without the envelope. Warnings have
		// no envelope to ride in, so fold any non-empty warning set into
		// the data so credential-cleanup and pagination notices stay
		// visible to agents (they would otherwise be silently dropped).
		return cli.WriteCompact(cmd.OutOrStdout(), foldRawWarningsIntoData(data, warnings))
	}
	if usePlainOutput(cmd) {
		return cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, plainOptionsForCommand(cmd)...)
	}
	body := map[string]any{
		"ok": true,
		"meta": map[string]any{
			"command":    command,
			"timestamp":  time.Now().UTC().Format(time.RFC3339),
			"request_id": cli.NewRequestID(),
		},
		"data":     data,
		"errors":   []any{},
		"warnings": rawWarningsOrEmpty(warnings),
	}
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(body)
}

// foldWarnings merges a non-empty warning slice into a compact-mode data
// payload so a correctness warning survives a mode that has no envelope
// to carry it. A map payload gets a "warnings" key alongside its existing
// fields; a non-map payload (slice or scalar) is wrapped as
// {"data": ..., "warnings": ...} so the warning is never silently
// dropped. An empty warning set returns the data unchanged.
func foldWarnings[T any](data any, warnings []T) any {
	if len(warnings) == 0 {
		return data
	}
	if m, ok := data.(map[string]any); ok {
		out := copyAnyMap(m)
		out["warnings"] = warnings
		return out
	}
	return map[string]any{"data": data, "warnings": warnings}
}

// foldRawWarningsIntoData folds a raw map-shaped warning slice into a
// compact payload. See foldWarnings.
func foldRawWarningsIntoData(data any, warnings []map[string]any) any {
	return foldWarnings(data, warnings)
}

// foldWarningsIntoData folds a typed cli.Warning slice into a compact
// payload. See foldWarnings.
func foldWarningsIntoData(data any, warnings []cli.Warning) any {
	return foldWarnings(data, warnings)
}

func rawWarningsOrEmpty(w []map[string]any) []map[string]any {
	if w == nil {
		return []map[string]any{}
	}
	return w
}

// writeEnvelopeWithWarnings is the warning-emitting envelope entry
// point — every command emitting structured warnings (typically from
// pipeline.RunMutation) calls this so warnings travel in the envelope
// under JSON mode and mirror to stderr under TTY/--plain (via the
// route helper).
func writeEnvelopeWithWarnings(cmd *cobra.Command, command string, data any, warnings []adf.Warning) error {
	cliWarnings := make([]cli.Warning, 0, len(warnings))
	for _, w := range warnings {
		cliWarnings = append(cliWarnings, cli.WarningFrom(w))
	}
	cliWarnings = append(cliWarnings, collectedCredentialWarnings(cmd)...)
	if useCompactOutput(cmd) {
		// compact has no envelope; fold warnings into the data so a failed
		// credential cleanup or other correctness notice is not lost.
		return cli.WriteCompact(cmd.OutOrStdout(), foldWarningsIntoData(data, cliWarnings))
	}
	if usePlainOutput(cmd) {
		// Data on stdout, warnings on stderr as clog WRN.
		if err := cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, plainOptionsForCommand(cmd)...); err != nil {
			return err
		}
		return mirrorADFWarningsToStderr(cmd.ErrOrStderr(), cliWarnings)
	}
	env := cli.Envelope{
		OK: true,
		Meta: cli.Meta{
			Command:   command,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			RequestID: cli.NewRequestID(),
		},
		Data:     data,
		Errors:   []cli.Error{},
		Warnings: cliWarnings,
	}
	if env.Warnings == nil {
		env.Warnings = []cli.Warning{}
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

func writeEnvelopeWithResponse(cmd *cobra.Command, command string, data any, resp *jira.Response) error {
	return writeEnvelopeWithResponseAndWarnings(cmd, command, data, resp, nil)
}

// writeEnvelopeWithResponseAndWarnings is the warning-emitting
// envelope entry point for commands that BOTH have a paginated/HTTP
// response AND need to surface
// pipeline warnings (e.g., live-submit issue create / edit / comment /
// worklog where the pipeline has already validated and produced
// best-effort warnings before the API call). Mirrors
// writeEnvelopeWithWarnings's TTY routing for plain mode so the data
// stays on stdout and warnings mirror to stderr as clog WRN lines.
func writeEnvelopeWithResponseAndWarnings(cmd *cobra.Command, command string, data any, resp *jira.Response, warnings []adf.Warning) error {
	if resp == nil {
		// writeEnvelopeWithWarnings collects the credential warnings itself.
		return writeEnvelopeWithWarnings(cmd, command, data, warnings)
	}
	cliWarnings := make([]cli.Warning, 0, len(warnings))
	for _, w := range warnings {
		cliWarnings = append(cliWarnings, cli.WarningFrom(w))
	}
	cliWarnings = append(cliWarnings, collectedCredentialWarnings(cmd)...)
	if useCompactOutput(cmd) {
		if m, ok := data.(map[string]any); ok {
			m["pagination"] = paginationFromResponse(resp)
			return cli.WriteCompact(cmd.OutOrStdout(), foldWarningsIntoData(m, cliWarnings))
		}
		return cli.WriteCompact(cmd.OutOrStdout(), data)
	}
	if usePlainOutput(cmd) {
		if err := cli.WriteCommandPlain(cmd.OutOrStdout(), command, data, plainOptionsForCommand(cmd)...); err != nil {
			return err
		}
		return mirrorADFWarningsToStderr(cmd.ErrOrStderr(), cliWarnings)
	}
	env := cli.Envelope{
		OK: true,
		Meta: cli.Meta{
			Command:    command,
			Timestamp:  time.Now().UTC().Format(time.RFC3339),
			RequestID:  cli.NewRequestID(),
			Pagination: paginationFromResponse(resp),
		},
		Data:     data,
		Errors:   []cli.Error{},
		Warnings: cliWarnings,
	}
	if env.Warnings == nil {
		env.Warnings = []cli.Warning{}
	}
	return cli.WriteEnvelope(cmd.OutOrStdout(), env)
}

// mirrorADFWarningsToStderr is a thin wrapper around cli.RouteWarnings
// for the plain-mode warning-only path used by both
// writeEnvelopeWithWarnings and writeEnvelopeWithResponseAndWarnings.
func mirrorADFWarningsToStderr(stderr io.Writer, warnings []cli.Warning) error {
	if len(warnings) == 0 || stderr == nil {
		return nil
	}
	return cli.RouteWarnings(cli.RouteOptions{
		Stderr:   stderr,
		Stdout:   io.Discard, // data was already written above
		Mode:     cli.RoutePlain,
		Command:  "",
		Data:     map[string]any{}, // no-op data, we only want the WRN lines
		Warnings: warnings,
	})
}

func plainOptionsForCommand(cmd *cobra.Command) []cli.PlainOption {
	det := DetectorFromContext(cmd)
	opts := []cli.PlainOption{
		cli.WithPlainTTY(det.IsTTY),
		cli.WithPlainTermWidth(terminal.Width(os.Stdout)),
	}
	if baseURL := plainBaseURL(cmd); baseURL != "" {
		opts = append(opts, cli.WithPlainBaseURL(baseURL))
	}
	return opts
}

func plainBaseURL(cmd *cobra.Command) string {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return ""
	}
	return activeProfile(cmd, cfg).BaseURL
}

func epicCommand() *cobra.Command {
	cmd := groupCommand("epic", "Work with Jira epics", "resources")
	cmd.AddCommand(epicListCommand())
	cmd.AddCommand(epicBoardCommand())
	cmd.AddCommand(epicAddCommand())
	cmd.AddCommand(epicRemoveCommand())
	return cmd
}

func epicListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List epics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				epics, resp, err := epicService(client).List(cmd.Context(), &jira.ListOptions{MaxResults: 50})
				if err != nil {
					return err
				}
				return writeEnvelopeWithResponse(cmd, "epic.list", map[string]any{"jql": "issuetype = Epic", "epics": epics, "detail": false}, resp)
			}
			return writeEnvelope(cmd, "epic.list", map[string]any{
				"jql":    "issuetype = Epic",
				"epics":  []any{},
				"detail": false,
			})
		},
	}
}

func epicBoardCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "Open the epic board",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if !ok {
				return writeEnvelope(cmd, "epic.board", map[string]any{
					"epics":  []any{},
					"totals": emptyEpicCounts(),
				})
			}
			service := epicService(client)
			epics, _, err := service.List(cmd.Context(), &jira.ListOptions{MaxResults: 50})
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(epics))
			totals := emptyEpicCounts()
			for _, epic := range epics {
				key := ""
				summary := ""
				status := ""
				if epic != nil && epic.Key != nil {
					key = *epic.Key
				}
				if epic != nil && epic.Fields != nil {
					if epic.Fields.Summary != nil {
						summary = *epic.Fields.Summary
					}
					if epic.Fields.Status != nil && epic.Fields.Status.Name != nil {
						status = *epic.Fields.Status.Name
					}
				}
				children, _, err := service.IssuesInEpic(cmd.Context(), key)
				if err != nil {
					return err
				}
				counts := jira.StatusCounts(children)
				for status, n := range counts {
					totals[status] += n
				}
				rows = append(rows, map[string]any{
					"key":     key,
					"summary": summary,
					"status":  status,
					"counts":  counts,
				})
			}
			return writeEnvelope(cmd, "epic.board", map[string]any{
				"epics":  rows,
				"totals": totals,
			})
		},
	}
}

func emptyEpicCounts() map[string]int {
	return map[string]int{"To Do": 0, "In Progress": 0, "Done": 0}
}

func epicAddCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "add ISSUE_KEY EPIC_KEY",
		Short: "Add an issue to an epic",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				client, _, ok, err := jiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					resp, err := epicService(client).AddIssue(cmd.Context(), args[1], args[0])
					if err != nil {
						return err
					}
					return writeEnvelopeWithResponse(cmd, "epic.add", map[string]any{"issue": args[0], "epic": args[1], "dry_run": false, "added": true}, resp)
				}
				return fmt.Errorf("jira base URL is required for epic.add")
			}
			return writeEnvelope(cmd, "epic.add", map[string]any{
				"issue":   args[0],
				"epic":    args[1],
				"dry_run": dryRun,
				"added":   !dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	return cmd
}

func epicRemoveCommand() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "remove ISSUE_KEY",
		Short: "Remove an issue from its epic",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !dryRun {
				client, _, ok, err := jiraClientForCommand(cmd)
				if err != nil {
					return err
				}
				if ok {
					resp, err := epicService(client).RemoveIssue(cmd.Context(), args[0])
					if err != nil {
						return err
					}
					return writeEnvelopeWithResponse(cmd, "epic.remove", map[string]any{"issue": args[0], "dry_run": false, "removed": true}, resp)
				}
				return fmt.Errorf("jira base URL is required for epic.remove")
			}
			return writeEnvelope(cmd, "epic.remove", map[string]any{
				"issue":   args[0],
				"dry_run": dryRun,
				"removed": !dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview mutation without submitting")
	return cmd
}

func searchCommand() *cobra.Command {
	cmd := groupCommand("search", "Run Jira searches", "resources")
	cmd.AddCommand(searchJQLCommand())
	cmd.AddCommand(searchSavedCommand())
	return cmd
}

func searchJQLCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "jql QUERY",
		Short: "Run a JQL query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				issues, resp, err := searchService(client).JQL(cmd.Context(), &jira.SearchRequest{JQL: args[0], ListOptions: jira.ListOptions{MaxResults: 50}})
				if err != nil {
					return err
				}
				return writeEnvelopeWithResponse(cmd, "search.jql", map[string]any{"source": "inline", "jql": args[0], "issues": issueOutput(issues, true)}, resp)
			}
			return writeEnvelope(cmd, "search.jql", map[string]any{
				"source": "inline",
				"jql":    args[0],
				"issues": []any{},
			})
		},
	}
}

func searchSavedCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "saved NAME",
		Short: "Run a saved JQL query",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			queries, err := config.LoadQueries(cfg.QueriesPath)
			if err != nil {
				return err
			}
			query, ok := queries[args[0]]
			if !ok {
				return fmt.Errorf("saved query %q not found", args[0])
			}
			client, _, hasClient, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			issues := any([]any{})
			var resp *jira.Response
			if hasClient {
				found, response, err := searchService(client).JQL(cmd.Context(), &jira.SearchRequest{JQL: query.JQL, ListOptions: jira.ListOptions{MaxResults: 50}})
				if err != nil {
					return err
				}
				issues = issueOutput(found, true)
				resp = response
			}
			data := map[string]any{
				"source":      "saved",
				"key":         args[0],
				"name":        query.Name,
				"description": query.Description,
				"project":     query.Project,
				"jql":         query.JQL,
				"issues":      issues,
			}
			return writeEnvelopeWithResponse(cmd, "search.saved", data, resp)
		},
	}
}

func worklogCommand() *cobra.Command {
	cmd := groupCommand("worklog", "Manage issue worklogs", "resources")
	cmd.AddCommand(worklogAddCommand())
	cmd.AddCommand(worklogListCommand())
	return cmd
}

func worklogListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list KEY",
		Short: "List worklogs",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, _, ok, err := jiraClientForCommand(cmd)
			if err != nil {
				return err
			}
			if ok {
				worklogs, resp, err := worklogService(client).List(cmd.Context(), args[0], &jira.ListOptions{MaxResults: 50})
				if err != nil {
					return err
				}
				return writeEnvelopeWithResponse(cmd, "worklog.list", map[string]any{"issue": args[0], "worklogs": worklogs}, resp)
			}
			return writeEnvelope(cmd, "worklog.list", map[string]any{
				"issue":    args[0],
				"worklogs": []any{},
			})
		},
	}
}

func configCommand() *cobra.Command {
	cmd := groupCommand("config", "Manage configuration", "configuration")
	cmd.AddCommand(configInitCommand())
	cmd.AddCommand(configProfileCommand())
	cmd.AddCommand(configGetCommand())
	cmd.AddCommand(configSetCommand())
	cmd.AddCommand(configThemeCommand())
	return cmd
}

func configThemeCommand() *cobra.Command {
	var name, path string
	cmd := &cobra.Command{
		Use:   "theme",
		Short: "Manage TUI theme configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			changed := false
			if cmd.Flags().Changed("name") {
				cfg.Theme.Name = name
				changed = true
			}
			if cmd.Flags().Changed("path") {
				cfg.Theme.Path = path
				changed = true
			}
			if changed {
				if err := cfg.Validate(); err != nil {
					return err
				}
				if err := config.Save(configPath(cmd), cfg); err != nil {
					return err
				}
			}
			return writeEnvelope(cmd, "config.theme", map[string]any{
				"name":    cfg.Theme.Name,
				"path":    cfg.Theme.Path,
				"changed": changed,
			})
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Theme name")
	cmd.Flags().StringVar(&path, "path", "", "Theme TOML path")
	return cmd
}

func configInitCommand() *cobra.Command {
	var baseURL, authType, email string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create initial configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			profile := requestedProfile(cmd)
			if profile == "" {
				profile = "default"
			}
			if authType == "" {
				authType = string(config.AuthTypeToken)
			}
			baseURL = config.NormalizeBaseURL(baseURL)
			cfg := config.Defaults()
			cfg.DefaultProfile = profile
			cfg.Profiles = []config.Profile{{
				Name:            profile,
				BaseURL:         baseURL,
				AuthType:        config.AuthType(authType),
				Email:           email,
				SecretBackend:   config.SecretBackendKeyring,
				RefreshInterval: config.DefaultRefreshIntervalSeconds,
				TimeoutSeconds:  config.DefaultTimeoutSeconds,
				WorkdaySeconds:  config.DefaultWorkdaySeconds,
			}}
			if err := cfg.Validate(); err != nil {
				return err
			}
			if err := config.Save(configPath(cmd), &cfg); err != nil {
				return err
			}
			return writeEnvelope(cmd, "config.init", map[string]any{
				"profile":     profile,
				"base_url":    baseURL,
				"auth_type":   authType,
				"stored_auth": false,
			})
		},
	}
	cmd.Flags().StringVar(&baseURL, "base-url", "", "Jira base URL")
	cmd.Flags().StringVar(&authType, "auth-type", "token", "Auth type")
	cmd.Flags().StringVar(&email, "email", "", "Jira account email")
	clib.Extend(cmd.Flags().Lookup("auth-type"), clib.FlagExtra{Placeholder: "TYPE", Enum: []string{"token", "basic", "pat", "mtls"}, EnumDefault: "token"})
	return cmd
}

func configProfileCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "profile",
		Short: "List configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			profiles := make([]map[string]any, 0, len(cfg.Profiles))
			for _, p := range cfg.Profiles {
				profiles = append(profiles, map[string]any{
					"name":   p.Name,
					"active": p.Name == cfg.DefaultProfile,
				})
			}
			return writeEnvelope(cmd, "config.profile", map[string]any{
				"active_profile": cfg.DefaultProfile,
				"profiles":       profiles,
			})
		},
	}
}

func configGetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "get KEY",
		Short:             "Show a configuration value",
		Args:              cobra.ExactArgs(1),
		Annotations:       map[string]string{"clib": "dynamic-args='configkey'"},
		ValidArgsFunction: completeConfigKeys,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			val, ok := cfg.Get(args[0])
			if !ok {
				return fmt.Errorf("unknown config key %q", args[0])
			}
			return writeEnvelope(cmd, "config.get", map[string]any{"key": args[0], "value": val})
		},
	}
}

func configSetCommand() *cobra.Command {
	return &cobra.Command{
		Use:               "set KEY VALUE",
		Short:             "Set a configuration value",
		Args:              cobra.ExactArgs(2),
		Annotations:       map[string]string{"clib": "dynamic-args='configkey,configvalue'"},
		ValidArgsFunction: completeConfigSetArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadOrInit(config.WithPath(configPath(cmd)))
			if err != nil {
				return err
			}
			if err := cfg.Set(args[0], args[1]); err != nil {
				return err
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			if err := config.Save(configPath(cmd), cfg); err != nil {
				return err
			}
			return writeEnvelope(cmd, "config.set", map[string]any{"key": args[0], "value": args[1]})
		},
	}
}

// completeProfileNames completes the names of all profiles in the config.
func completeProfileNames(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	out := make([]string, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		out = append(out, p.Name)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeConfigKeys lists every valid config key (profile-scoped keys
// expanded for each present profile) along with its description. Falls back
// to template form when the config can't be loaded.
func completeConfigKeys(cmd *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cfg, _ := config.Load(config.WithPath(configPath(cmd)))
	keys := config.Keys(cfg)
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = k.Name + "\t" + k.Description
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// completeConfigSetArgs completes both positional args of `config set`:
// arg 0 is the key list (same as `get`); arg 1 is the value enum for
// closed-set keys, or no completion for freeform values.
func completeConfigSetArgs(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeConfigKeys(cmd, args, "")
	case 1:
		if choices := config.KeyChoices(args[0]); len(choices) > 0 {
			return choices, cobra.ShellCompDirectiveNoFileComp
		}
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func configPath(cmd *cobra.Command) string {
	path, _ := cmd.Root().PersistentFlags().GetString("config")
	return path
}

func groupCommand(use, short, group string) *cobra.Command {
	return &cobra.Command{
		Use:     use,
		Short:   short,
		GroupID: group,
	}
}

func requestedProfile(cmd *cobra.Command) string {
	profile, _ := cmd.Root().PersistentFlags().GetString("profile")
	return profile
}

// resolvedOutputMode returns the output mode resolved by PersistentPreRunE
// from the --output flag and terminal/agent detection. It is the single
// source of truth for every command output helper.
func resolvedOutputMode(cmd *cobra.Command) cli.Mode {
	return DetectorFromContext(cmd).Mode
}

func useCompactOutput(cmd *cobra.Command) bool {
	return resolvedOutputMode(cmd) == cli.ModeCompact
}

func usePlainOutput(cmd *cobra.Command) bool {
	mode := resolvedOutputMode(cmd)
	return mode == cli.ModePlain || mode == cli.ModeTUI
}

func workdaySecondsForCommand(cmd *cobra.Command) int {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return config.DefaultWorkdaySeconds
	}
	profile := activeProfile(cmd, cfg)
	if profile.WorkdaySeconds <= 0 {
		return config.DefaultWorkdaySeconds
	}
	return profile.WorkdaySeconds
}

func activeProfile(cmd *cobra.Command, cfg *config.Config) config.Profile {
	return cfg.Profile(requestedProfile(cmd))
}

// profileForCommand resolves the active profile for a command WITHOUT
// constructing a Jira client or touching any credential backend. Local
// preview and dry-run paths use this so a validation-only run cannot fail
// on a locked keyring or an offline 1Password backend. Commands that make
// live HTTP calls must still go through jiraClientForCommand.
func profileForCommand(cmd *cobra.Command) (config.Profile, error) {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return config.Profile{}, err
	}
	return cfg.ResolveProfile(requestedProfile(cmd))
}

func profileForEnvelope(cmd *cobra.Command) string {
	if profile := requestedProfile(cmd); profile != "" {
		return profile
	}
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil || cfg.DefaultProfile == "" {
		return "default"
	}
	return cfg.DefaultProfile
}

func credentialStoreFor(backend config.SecretBackend) config.CredentialStore {
	if backend == config.SecretBackendOnePassword {
		return config.OnePasswordStore{}
	}
	return config.KeyringStore{}
}

// credentialWarnSink is a per-command collector for credential notices (a
// migration cleanup note, a kept user-named 1Password item on logout, an
// orphaned credential after a site change). It is installed into the command
// context by PersistentPreRunE, so each command invocation owns a fresh,
// isolated sink — a notice raised by one command can never reach another.
type credentialWarnSink struct {
	mu    sync.Mutex
	warns []string
}

func (s *credentialWarnSink) add(warns []string) {
	if len(warns) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range warns {
		if !slices.Contains(s.warns, w) {
			s.warns = append(s.warns, w)
		}
	}
}

func (s *credentialWarnSink) collected() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.warns)
}

// withCredentialWarnSink returns a context carrying a fresh credential-warning
// sink. PersistentPreRunE installs one per command invocation.
func withCredentialWarnSink(ctx context.Context) context.Context {
	return context.WithValue(ctx, credentialWarnSinkKey, &credentialWarnSink{})
}

// recordCredentialWarnings appends resolution warnings to the command's sink,
// if one is installed. Commands without a sink (direct test calls) drop the
// warnings silently — they are diagnostics, not results.
func recordCredentialWarnings(cmd *cobra.Command, warns []string) {
	if len(warns) == 0 || cmd == nil {
		return
	}
	if sink, ok := cmd.Context().Value(credentialWarnSinkKey).(*credentialWarnSink); ok {
		sink.add(warns)
	}
}

// collectedCredentialWarnings returns the credential warnings recorded for the
// current command as envelope warnings. The sink is per-command, so this
// only ever returns warnings from the command currently executing.
func collectedCredentialWarnings(cmd *cobra.Command) []cli.Warning {
	if cmd == nil {
		return nil
	}
	sink, ok := cmd.Context().Value(credentialWarnSinkKey).(*credentialWarnSink)
	if !ok {
		return nil
	}
	return credentialWarningsToEnvelope(sink.collected())
}

// credentialWarningsToEnvelope renders credential notices — a migration
// cleanup note, a kept user-named 1Password item, an orphaned credential
// after a site change — as envelope warnings under one informational type.
func credentialWarningsToEnvelope(msgs []string) []cli.Warning {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]cli.Warning, 0, len(msgs))
	for _, msg := range msgs {
		out = append(out, cli.Warning{
			Type:    "credential_notice",
			Message: msg,
			Lossy:   false,
		})
	}
	return out
}

// existingProfileOrDefault returns a copy of the named profile from cfg if it
// exists, or a new Profile with the given name. Used by authLoginCommand to
// merge a partial update instead of wholesale replacing the persisted profile
// (: preserves fields not supplied to `auth login`).
func existingProfileOrDefault(cfg *config.Config, name string) config.Profile {
	for _, p := range cfg.Profiles {
		if p.Name == name {
			return p
		}
	}
	return config.Profile{Name: name}
}

func upsertProfile(cfg *config.Config, profile config.Profile) {
	for i := range cfg.Profiles {
		if cfg.Profiles[i].Name == profile.Name {
			cfg.Profiles[i] = profile
			return
		}
	}
	cfg.Profiles = append(cfg.Profiles, profile)
}

// profileBaseURLEnvVar returns the name of the environment variable that
// overrides a profile's base_url. It mirrors the JIRA_PROFILE_<NAME>_*
// convention parsed by the config loader: the profile name is uppercased
// with '-' replaced by '_'.
func profileBaseURLEnvVar(name string) string {
	return "JIRA_PROFILE_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_")) + "_BASE_URL"
}

// secretRefFor derives the credential identity for a profile under a given
// backend. The credential is keyed by the profile's Jira site host and name;
// an unsafe profile name is rejected here rather than producing a malformed
// keyring entry.
func secretRefFor(profile config.Profile, backend config.SecretBackend) (config.SecretRef, error) {
	scoped := profile
	scoped.SecretBackend = backend
	return config.CredentialIdentity(scoped)
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
	oldRef, err := secretRefFor(previous, previous.SecretBackend)
	if err != nil {
		return ""
	}
	newRef, err := secretRefFor(updated, updated.SecretBackend)
	if err != nil {
		return ""
	}
	return revokeOldCredential(cmd.Context(), credentialStoreFor(previous.SecretBackend), oldRef, newRef)
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

func jiraClientForCommand(cmd *cobra.Command) (*jira.Client, config.Profile, bool, error) {
	cfg, err := config.Load(config.WithPath(configPath(cmd)))
	if err != nil {
		return nil, config.Profile{}, false, err
	}
	return jiraClientForProfile(cmd, activeProfile(cmd, cfg))
}

// jiraClientForProfile builds a Jira client targeting an explicit profile
// rather than the env-overlaid active profile. Read-modify-write commands
// that persist server data (`auth whoami --save`) use this so the live
// request and the saved record come from the same file-backed profile: a
// JIRA_PROFILE_*_BASE_URL overlay cannot redirect the request to another
// tenant whose identity would then be written into the file profile.
// Credential env sources (token/password env vars) are still honored.
func jiraClientForProfile(cmd *cobra.Command, profile config.Profile) (*jira.Client, config.Profile, bool, error) {
	if profile.BaseURL == "" {
		return nil, profile, false, nil
	}
	debug, _ := cmd.Root().PersistentFlags().GetBool("debug")
	opts := []jira.Option{
		jira.WithBaseURL(profile.BaseURL),
		jira.WithHTTPClient(&http.Client{Timeout: time.Duration(profile.TimeoutSeconds) * time.Second}),
		// Single source of truth for the read-only gate. Set on the client
		// so EVERY mutation across EVERY command is automatically refused
		// without per-command boilerplate that's easy to forget.
		jira.WithReadOnly(readOnlyEnabled(cmd)),
		// Service-level dry-run guard: when --dry-run is set, the client
		// refuses every state-changing request. Defense in depth behind
		// the command-layer dry-run branches.
		jira.WithDryRun(dryRunRequested(cmd)),
		jira.WithDebug(debug),
	}
	if profile.AuthType == config.AuthTypeMTLS {
		if profile.MTLSCertRef == "" || profile.MTLSKeyRef == "" {
			return nil, profile, false, fmt.Errorf("mTLS profile %q requires mtls_cert_ref and mtls_key_ref", profile.Name)
		}
		httpClient, err := jira.MTLSHTTPClient(profile.MTLSCertRef, profile.MTLSKeyRef, time.Duration(profile.TimeoutSeconds)*time.Second)
		if err != nil {
			return nil, profile, false, err
		}
		opts = append(opts, jira.WithHTTPClient(httpClient))
	} else {
		ref, refErr := secretRefFor(profile, profile.SecretBackend)
		if refErr != nil {
			return nil, profile, false, refErr
		}
		secret, secretErr := config.ResolveCredential(cmd.Context(), credentialStoreFor(profile.SecretBackend), ref)
		if secretErr != nil && !isLocalBaseURL(profile.BaseURL) {
			return nil, profile, false, fmt.Errorf("credential for profile %q is required: %w", profile.Name, secretErr)
		}
		if secret != "" {
			switch profile.AuthType {
			case config.AuthTypeBasic, config.AuthTypeToken:
				opts = append(opts, jira.WithBasicAuth(firstNonEmpty(profile.Email, profile.Username), secret))
			case config.AuthTypePAT:
				opts = append(opts, jira.WithBearerToken(secret))
			}
		}
	}
	return jira.NewClient(opts...), profile, true, nil
}

func isLocalBaseURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func paginationFromResponse(resp *jira.Response) *cli.Pagination {
	if resp == nil {
		return nil
	}
	return &cli.Pagination{
		StartAt:    resp.StartAt, // pagination-exempt: output-shape, not consumer cursor
		MaxResults: resp.MaxResults,
		Total:      resp.Total,
		IsLast:     resp.NextCursor() == "",
		NextCursor: resp.NextCursor(),
	}
}

func issueOutput(issues []*jira.Issue, detail bool) any {
	if detail {
		return issues
	}
	out := make([]map[string]any, 0, len(issues))
	for _, issue := range issues {
		out = append(out, issueSummary(issue))
	}
	return out
}

func issueSummary(issue *jira.Issue) map[string]any {
	summary := map[string]any{
		"key":      "",
		"summary":  "",
		"status":   "",
		"assignee": nil,
		"priority": nil,
		"updated":  "",
	}
	if issue == nil {
		return summary
	}
	if issue.Key != nil {
		summary["key"] = *issue.Key
	}
	if issue.Fields == nil {
		return summary
	}
	if issue.Fields.Summary != nil {
		summary["summary"] = *issue.Fields.Summary
	}
	if issue.Fields.Status != nil && issue.Fields.Status.Name != nil {
		summary["status"] = *issue.Fields.Status.Name
	}
	if user := issue.Fields.Assignee; user != nil {
		summary["assignee"] = assigneeSummary(user)
	}
	if issue.Fields.Priority != nil && issue.Fields.Priority.Name != nil {
		summary["priority"] = *issue.Fields.Priority.Name
	}
	if issue.Fields.Updated != nil {
		summary["updated"] = *issue.Fields.Updated
	}
	return summary
}

func assigneeSummary(user *jira.User) map[string]any {
	out := map[string]any{
		"account_id":   "",
		"display_name": "",
	}
	if user.AccountID != nil {
		out["account_id"] = *user.AccountID
	}
	if user.DisplayName != nil {
		out["display_name"] = *user.DisplayName
	}
	return out
}

func readJSONFile(path string, dst any) error {
	r, err := stdininput.JSONInput(path)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()
	dec := json.NewDecoder(r)
	if err := dec.Decode(dst); err != nil {
		return err
	}
	// A --json-input file must hold exactly one JSON document. Decode a
	// second value into a throwaway target: io.EOF is the only acceptable
	// result. Anything else — a second value, a stray trailing `}`/`]`, or
	// a syntax error — means a malformed or concatenated payload. A
	// Decoder.More() check is insufficient: More() reports false for a
	// trailing structural byte, letting `{"summary":"ok"}}` through.
	var trailing json.RawMessage
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("invalid json input %q: unexpected data after the JSON value", path)
	}
	return nil
}

func stringFromAny(v any) string {
	switch v := v.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// extractNamedADFDocs walks a payload map, finds every value whose JSON shape
// is an ADF root document ({"type":"doc","version":N,"content":[...]}), parses
// it, and returns a map keyed by the original field name. Detection is by
// value shape, not key suffix — Jira's API expects ADF under bare field names
// (`description`, `environment`, `customfield_NNNN`), and a key-suffix
// convention (`description_adf`) would be forwarded verbatim and rejected by
// Jira as an unknown field. The CLI now validates whichever ADF-shaped values
// the caller supplies under their actual destination key.
func extractNamedADFDocs(payload map[string]any) (map[string]adf.Document, error) {
	var named map[string]adf.Document
	for k, v := range payload {
		if !looksLikeADFDoc(v) {
			continue
		}
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("marshal %s for ADF validation: %w", k, err)
		}
		doc, _, err := adf.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		if named == nil {
			named = make(map[string]adf.Document)
		}
		named[k] = doc
	}
	return named, nil
}

// looksLikeADFDoc reports whether v is an object whose top-level shape is an
// ADF root document — type field equals "doc" and a version field is present.
// Cheap shape gate before the full marshal+parse path; avoids parsing every
// nested object (e.g. assignee={accountId:...}, project={key:...}) as ADF.
func looksLikeADFDoc(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	t, _ := m["type"].(string)
	if t != "doc" {
		return false
	}
	_, hasVersion := m["version"]
	return hasVersion
}
