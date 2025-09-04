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

package runner

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	//
	"github.com/Nerzal/gocloak/v13"
	"kegos/internal/globals"
	"kegos/internal/gsuite"
	"kegos/internal/keycloak"
)

type RunnerOptions struct {
	AppCtx *globals.ApplicationContext

	GsuiteJsonCredentialsPath string
	GsuiteDomain              string

	KeycloakURI          string
	KeycloakRealm        string
	KeycloakClientID     string
	KeycloakClientSecret string

	ReconcileLoopDuration time.Duration
	SyncedParentGroup     string
}

type Runner struct {
	appCtx *globals.ApplicationContext

	//
	gsuiteJsonCredentialsPath string
	gsuiteDomain              string

	//
	keycloakURI          string
	keycloakRealm        string
	keycloakClientID     string
	keycloakClientSecret string

	//
	reconcileLoopDuration time.Duration
	syncedParentGroup     string

	//
	gsuiteCli     *gsuite.Admin
	keycloakCli   *gocloak.GoCloak
	keycloakToken *gocloak.JWT
}

func NewRunner(opts RunnerOptions) (*Runner, error) {

	runner := &Runner{
		appCtx:                    opts.AppCtx,
		gsuiteJsonCredentialsPath: opts.GsuiteJsonCredentialsPath,
		gsuiteDomain:              opts.GsuiteDomain,
		keycloakURI:               opts.KeycloakURI,
		keycloakRealm:             opts.KeycloakRealm,
		keycloakClientID:          opts.KeycloakClientID,
		keycloakClientSecret:      opts.KeycloakClientSecret,
		reconcileLoopDuration:     opts.ReconcileLoopDuration,
		syncedParentGroup:         opts.SyncedParentGroup,
	}

	gsuiteCli, err := gsuite.NewAdmin(context.Background(), runner.gsuiteJsonCredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating gsuite client: %v", err)

	}

	kcClient := gocloak.NewClient(runner.keycloakURI)

	// A Keycloak client with Service Account flow enabled with enough permissions is needed
	kcToken, err := kcClient.LoginClient(runner.appCtx.Context, runner.keycloakClientID, runner.keycloakClientSecret, runner.keycloakRealm)
	if err != nil {
		return nil, fmt.Errorf("failed signing in in Keycloak: %v", err)
	}

	runner.gsuiteCli = &gsuiteCli
	runner.keycloakCli = kcClient
	runner.keycloakToken = kcToken

	return runner, nil
}

// getKeycloakChildrenGroups TODO
func (r *Runner) getKeycloakChildrenGroups() (parentGroup *string, childrenGroups map[string]*gocloak.Group, err error) {

	// 1. Retrieve Keycloak groups
	kcExistingGroups, err := r.keycloakCli.GetGroups(r.appCtx.Context, r.keycloakToken.AccessToken, r.keycloakRealm, gocloak.GetGroupsParams{
		Full:   gocloak.BoolP(true),
		Exact:  gocloak.BoolP(true),
		Max:    gocloak.IntP(1),
		Search: gocloak.StringP(r.syncedParentGroup),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed getting groups: %v", err)
	}

	// 2. Retrieve children groups for the found parent.
	// When the parent is not found, create it
	kcParentGroup := gocloak.Group{}
	kcChildrenGroups := []*gocloak.Group{}

	if len(kcExistingGroups) == 0 {
		kcParentGroup.Name = gocloak.StringP(r.syncedParentGroup)

		gCreationResult, err := r.keycloakCli.CreateGroup(r.appCtx.Context, r.keycloakToken.AccessToken, r.keycloakRealm, kcParentGroup)
		if err != nil {
			return nil, nil, fmt.Errorf("failed creating parent group: %v", err)
		}

		kcParentGroup.ID = gocloak.StringP(gCreationResult)
	} else {
		kcParentGroup = *kcExistingGroups[0]
	}

	kcChildrenGroups, err = keycloak.GetChildrenGroups(r.appCtx.Context, r.keycloakURI,
		r.keycloakRealm, *kcParentGroup.ID, r.keycloakToken.AccessToken)
	if err != nil {
		return nil, nil, fmt.Errorf("failed getting children groups: %v", err)
	}

	kcChildrenGroupsMap := map[string]*gocloak.Group{}
	for _, kcGroup := range kcChildrenGroups {
		kcChildrenGroupsMap[*kcGroup.Name] = kcGroup
	}

	return kcParentGroup.ID, kcChildrenGroupsMap, nil
}

// KeycloakUserGroups represents the merge between a user and its groups
type KeycloakUserGroups struct {
	User   *gocloak.User
	Groups map[string]*gocloak.Group
}

// getKeycloakUsersGroups TODO
func (r *Runner) getKeycloakUsersGroups() (usersGroups map[string]KeycloakUserGroups, err error) {

	kcUsersGroups := map[string]KeycloakUserGroups{}

	kcUsers, err := r.keycloakCli.GetUsers(r.appCtx.Context, r.keycloakToken.AccessToken, r.keycloakRealm, gocloak.GetUsersParams{})
	if err != nil {
		return nil, fmt.Errorf("failed getting users: %v", err)
	}

	// Create a map to merge a user and its groups into a unique object.
	for _, user := range kcUsers {

		kcUserGroups, err := r.keycloakCli.GetUserGroups(r.appCtx.Context, r.keycloakToken.AccessToken, r.keycloakRealm, *user.ID, gocloak.GetGroupsParams{})
		if err != nil {
			r.appCtx.Logger.Error("failed getting user groups. Ignoring user...", "user", *user.Email, "error", err)
			continue
		}

		tmpGroupsMap := map[string]*gocloak.Group{}
		for _, kcGroup := range kcUserGroups {
			tmpGroupsMap[*kcGroup.Name] = kcGroup
		}

		kcUsersGroups[*user.Username] = KeycloakUserGroups{
			User:   user,
			Groups: tmpGroupsMap,
		}
	}

	return kcUsersGroups, nil
}

// TODO
func (r *Runner) reconcileUserGroups() {

	// 1. Retrieve Keycloak groups
	kcParentGroupID, kcChildrenGroups, err := r.getKeycloakChildrenGroups()
	if err != nil {
		r.appCtx.Logger.Error("failed getting groups from Keycloak", "error", err.Error())
		return
	}

	// 2. Get users groups in a map like: username->{userProfile, userGroups}
	kcUsersGroupsMap, err := r.getKeycloakUsersGroups()
	if err != nil {
		r.appCtx.Logger.Error("failed getting users groups from Keycloak", "error", err.Error())
		return
	}

	// 3. Reconcile group memberships in Keycloak having Gsuite as source of truth.
	for kcUsername, kcUserGroups := range kcUsersGroupsMap {

		r.appCtx.Logger.Info("reconciling user groups", "user", kcUsername)

		gsuiteGroups, err := r.gsuiteCli.GetGroupsFromUser(r.gsuiteDomain, kcUsername)
		if err != nil {
			r.appCtx.Logger.Error("failed getting groups from Gsuite. Ignoring user...", "user", kcUsername, "error", err.Error())
			continue
		}

		// Deletions
		// Groups attached in Keycloak and not attached in Gsuite
		// will be deleted. This is only true for auto-managed groups
		for _, kcUserGroup := range kcUserGroups.Groups {

			// Ignore not auto-managed groups
			if !strings.HasPrefix(*kcUserGroup.Path, "/"+r.syncedParentGroup) {
				continue
			}

			// Existing groups not present in Google
			if !slices.Contains(gsuiteGroups, *kcUserGroup.Name) {

				r.appCtx.Logger.Debug("deleting user from group", "user", kcUsername, "group", *kcUserGroup.Name)

				delUserGroupErr := r.keycloakCli.DeleteUserFromGroup(r.appCtx.Context, r.keycloakToken.AccessToken,
					r.keycloakRealm, *kcUserGroups.User.ID, *kcChildrenGroups[*kcUserGroup.Name].ID)

				if delUserGroupErr != nil {
					r.appCtx.Logger.Error("failed deleting user from group", "user", kcUsername,
						"group", *kcUserGroup.Name, "error", delUserGroupErr.Error())
				}
			}
		}

		// Additions
		// Groups attached in Gsuite and not attached in Keycloak
		// will be attached in Keycloak
		for _, gsuiteGroup := range gsuiteGroups {

			// Ignore user groups from Gsuite that are already present in Keycloak user profile
			_, groupFound := kcUserGroups.Groups[gsuiteGroup]
			if groupFound {
				continue
			}

			//
			tmpGroup := &gocloak.Group{
				Name: gocloak.StringP(gsuiteGroup),
			}

			_, groupFoundInGlobalMap := kcChildrenGroups[*tmpGroup.Name]
			if !groupFoundInGlobalMap {
				r.appCtx.Logger.Debug("creating missing group in Keycloak", "group", *tmpGroup.Name)

				childGroupID, err := r.keycloakCli.CreateChildGroup(r.appCtx.Context, r.keycloakToken.AccessToken, r.keycloakRealm,
					*kcParentGroupID, *tmpGroup)

				if err != nil {
					r.appCtx.Logger.Error("failed creating group in Keycloak", "group", *tmpGroup.Name, "error", err.Error())

					// When group creation fail, we don't want this membership to be added to the user.
					// It would also fail.
					continue
				}

				tmpGroup.ID = &childGroupID
				kcChildrenGroups[*tmpGroup.Name] = tmpGroup
			}

			r.appCtx.Logger.Debug("adding user to group", "user", kcUsername, "group", *tmpGroup.Name)
			addUserGroupErr := r.keycloakCli.AddUserToGroup(r.appCtx.Context, r.keycloakToken.AccessToken, r.keycloakRealm,
				*kcUserGroups.User.ID, *kcChildrenGroups[*tmpGroup.Name].ID)

			if addUserGroupErr != nil {
				r.appCtx.Logger.Error("failed adding user to the group",
					"user", kcUsername, "group", *tmpGroup.Name, "error", addUserGroupErr.Error())
			}
		}

	}
}

func (r *Runner) PleaseDoYourStuffForever() {
	for {
		r.reconcileUserGroups()

		r.appCtx.Logger.Info(fmt.Sprintf("reconcile group finished. waiting for the next loop in %s", r.reconcileLoopDuration.String()))
		time.Sleep(r.reconcileLoopDuration)
	}
}
