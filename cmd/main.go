// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	//
	"kegos/internal/globals"
	"kegos/internal/runner"
)

var (
	flagGsuiteCredentials    = flag.String("gsuite-credentials", "", "Path to GSuite JSON credentials file (required)")
	flagResolveAliases       = flag.Bool("resolve-aliases", false, "Resolve each Keycloak username to its Google primary email before syncing groups")
	flagUserRateLimit        = flag.Int("user-rate-limit", 60, "Max users processed per minute against the Google API (0 disables throttling)")
	flagKeycloakRealm        = flag.String("keycloak-realm", "", "Keycloak realm (required)")
	flagKeycloakURI          = flag.String("keycloak-uri", "", "Keycloak URI (required)")
	flagKeycloakClientID     = flag.String("keycloak-client-id", "", "Keycloak client ID (required)")
	flagKeycloakClientSecret = flag.String("keycloak-client-secret", "", "Keycloak client secret (required)")
	flagReconcileInterval    = flag.Duration("reconcile-interval", 10*time.Minute, "Reconcile loop duration")
	flagSyncedParentGroup    = flag.String("synced-parent-group", "", "Keycloak group where to sync Gsuite groups")
	flagLogLevel             = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	help                     = flag.Bool("help", false, "Show help")
)

// getValueFromFlagOrEnv returns the value from flag if not empty, otherwise from environment variable
func getValueFromFlagOrEnv(flagValue *string, envVar string) string {
	if *flagValue != "" {
		return *flagValue
	}
	return os.Getenv(envVar)
}

// flagWasSet reports whether the named flag was explicitly provided on the command line.
func flagWasSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}

// resolveBool applies flag-over-env precedence for a bool: an explicit flag wins, otherwise a
// parseable env var, otherwise the flag default.
func resolveBool(flagSet bool, flagValue bool, envRaw string) bool {
	if flagSet {
		return flagValue
	}
	if parsed, err := strconv.ParseBool(envRaw); err == nil {
		return parsed
	}
	return flagValue
}

// resolveInt applies flag-over-env precedence for an int: an explicit flag wins, otherwise a
// parseable env var, otherwise the flag default.
func resolveInt(flagSet bool, flagValue int, envRaw string) int {
	if flagSet {
		return flagValue
	}
	if parsed, err := strconv.Atoi(envRaw); err == nil {
		return parsed
	}
	return flagValue
}

func main() {

	flag.Parse()

	// Show help when required
	if *help {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Printf("\nEnvironment Variables (override flags):\n")
		fmt.Printf("  GSUITE_CREDENTIALS     - Path to GSuite JSON credentials file\n")
		fmt.Printf("  KEYCLOAK_REALM         - Keycloak realm\n")
		fmt.Printf("  KEYCLOAK_URI           - Keycloak URI\n")
		fmt.Printf("  KEYCLOAK_CLIENT_ID     - Keycloak client ID\n")
		fmt.Printf("  KEYCLOAK_CLIENT_SECRET - Keycloak client secret\n")
		fmt.Printf("  LOG_LEVEL              - Log level (debug, info, warn, error)\n")
		fmt.Printf("  SYNCED_PARENT_GROUP    - Keycloak group where to sync Gsuite groups\n")
		fmt.Printf("  RESOLVE_ALIASES        - Resolve usernames to their Google primary email (true/false)\n")
		fmt.Printf("  USER_RATE_LIMIT        - Max users processed per minute against the Google API\n")

		os.Exit(0)
	}

	// Get final values from flags or environment variables
	gsuiteCredentials := getValueFromFlagOrEnv(flagGsuiteCredentials, "GSUITE_CREDENTIALS")
	keycloakRealm := getValueFromFlagOrEnv(flagKeycloakRealm, "KEYCLOAK_REALM")
	keycloakURI := getValueFromFlagOrEnv(flagKeycloakURI, "KEYCLOAK_URI")
	keycloakClientID := getValueFromFlagOrEnv(flagKeycloakClientID, "KEYCLOAK_CLIENT_ID")
	keycloakClientSecret := getValueFromFlagOrEnv(flagKeycloakClientSecret, "KEYCLOAK_CLIENT_SECRET")
	logLevel := getValueFromFlagOrEnv(flagLogLevel, "LOG_LEVEL")
	syncedParentGroup := getValueFromFlagOrEnv(flagSyncedParentGroup, "SYNCED_PARENT_GROUP")
	resolveAliases := resolveBool(flagWasSet("resolve-aliases"), *flagResolveAliases, os.Getenv("RESOLVE_ALIASES"))
	userRateLimit := resolveInt(flagWasSet("user-rate-limit"), *flagUserRateLimit, os.Getenv("USER_RATE_LIMIT"))

	// Validate flags compliance
	var errors []string

	if gsuiteCredentials == "" {
		errors = append(errors, "--gsuite-credentials is required")
	}
	if keycloakRealm == "" {
		errors = append(errors, "--keycloak-realm is required")
	}
	if keycloakURI == "" {
		errors = append(errors, "--keycloak-uri is required")
	}
	if keycloakClientID == "" {
		errors = append(errors, "--keycloak-client-id is required")
	}
	if keycloakClientSecret == "" {
		errors = append(errors, "--keycloak-client-secret is required")
	}

	if syncedParentGroup == "" {
		errors = append(errors, "--synced-parent-group is required")
	}

	_, levelFound := globals.LogLevelMap[*flagLogLevel]
	if !levelFound {
		errors = append(errors, "--log-level must be one of: debug, info, warn, error")
	}

	// Validate edge cases
	if *flagReconcileInterval <= 0 {
		errors = append(errors, "--reconcile-interval must be positive")
	}

	// Quit on errors
	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Error: Invalid arguments:\n")
		for _, err := range errors {
			fmt.Fprintf(os.Stderr, "  * %s\n", err)
		}
		fmt.Fprintf(os.Stderr, "\nUse --help for usage information.\n")
		os.Exit(1)
	}

	//
	if _, err := os.Stat(gsuiteCredentials); os.IsNotExist(err) {
		log.Fatalf("GSuite credentials file does not exist: %s", gsuiteCredentials)
	}

	//
	appCtx, err := globals.NewApplicationContext(globals.ApplicationContextOptions{
		LogLevel: logLevel,
	})
	if err != nil {
		log.Fatalf("failed creating application context: %v", err.Error())
	}

	// 1. Launch the runner
	leRunner, err := runner.NewRunner(runner.RunnerOptions{
		AppCtx:                    appCtx,
		GsuiteJsonCredentialsPath: gsuiteCredentials,
		ResolveAliases:            resolveAliases,
		UserRateLimit:             userRateLimit,
		KeycloakRealm:             keycloakRealm,
		KeycloakURI:               keycloakURI,
		KeycloakClientID:          keycloakClientID,
		KeycloakClientSecret:      keycloakClientSecret,
		ReconcileLoopDuration:     *flagReconcileInterval,
		SyncedParentGroup:         syncedParentGroup,
	})
	if err != nil {
		log.Fatalf("failed creating runner: %v", err.Error())
	}

	leRunner.PleaseDoYourStuffForever()
}
