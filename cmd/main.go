/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	//
	"kegos/internal/globals"
	"kegos/internal/runner"
)

var (
	gsuiteCredentials    = flag.String("gsuite-credentials", "", "Path to GSuite JSON credentials file (required)")
	gsuiteDomain         = flag.String("gsuite-domain", "", "GSuite domain (required)")
	keycloakRealm        = flag.String("keycloak-realm", "", "Keycloak realm (required)")
	keycloakURI          = flag.String("keycloak-uri", "", "Keycloak URI (required)")
	keycloakClientID     = flag.String("keycloak-client-id", "", "Keycloak client ID (required)")
	keycloakClientSecret = flag.String("keycloak-client-secret", "", "Keycloak client secret (required)")
	reconcileInterval    = flag.Duration("reconcile-interval", 10*time.Minute, "Reconcile loop duration")
	syncedParentGroup    = flag.String("synced-parent-group", "", "Keycloak group where to sync Gsuite groups")
	logLevel             = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	help                 = flag.Bool("help", false, "Show help")
)

func main() {

	flag.Parse()

	// Show help when required
	if *help {
		fmt.Printf("Usage of %s:\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Validate flags compliance
	var errors []string

	if *gsuiteCredentials == "" {
		errors = append(errors, "--gsuite-credentials is required")
	}
	if *gsuiteDomain == "" {
		errors = append(errors, "--gsuite-domain is required")
	}
	if *keycloakRealm == "" {
		errors = append(errors, "--keycloak-realm is required")
	}
	if *keycloakURI == "" {
		errors = append(errors, "--keycloak-uri is required")
	}
	if *keycloakClientID == "" {
		errors = append(errors, "--keycloak-client-id is required")
	}
	if *keycloakClientSecret == "" {
		errors = append(errors, "--keycloak-client-secret is required")
	}

	if *syncedParentGroup == "" {
		errors = append(errors, "--synced-parent-group is required")
	}

	_, levelFound := globals.LogLevelMap[*logLevel]
	if !levelFound {
		errors = append(errors, "--log-level must be one of: debug, info, warn, error")
	}

	// Validate edge cases
	if *reconcileInterval <= 0 {
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
	if _, err := os.Stat(*gsuiteCredentials); os.IsNotExist(err) {
		log.Fatalf("GSuite credentials file does not exist: %s", *gsuiteCredentials)
	}

	//
	appCtx, err := globals.NewApplicationContext(globals.ApplicationContextOptions{
		LogLevel: *logLevel,
	})
	if err != nil {
		log.Fatalf("failed creating application context: %v", err.Error())
	}

	// 1. Launch the runner
	leRunner, err := runner.NewRunner(runner.RunnerOptions{
		AppCtx:                    appCtx,
		GsuiteJsonCredentialsPath: *gsuiteCredentials,
		GsuiteDomain:              *gsuiteDomain,
		KeycloakRealm:             *keycloakRealm,
		KeycloakURI:               *keycloakURI,
		KeycloakClientID:          *keycloakClientID,
		KeycloakClientSecret:      *keycloakClientSecret,
		ReconcileLoopDuration:     *reconcileInterval,
		SyncedParentGroup:         *syncedParentGroup,
	})
	if err != nil {
		log.Fatalf("failed creating runner: %v", err.Error())
	}

	leRunner.PleaseDoYourStuffForever()
}
