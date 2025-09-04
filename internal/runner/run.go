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
	reconcileLoopDuration time.Duration
	syncedParentGroup     string

	//
	gsuiteCli *gsuite.Admin
	keycloak  *keycloak.Keycloak
}

func NewRunner(opts RunnerOptions) (*Runner, error) {

	runner := &Runner{
		appCtx:                    opts.AppCtx,
		gsuiteJsonCredentialsPath: opts.GsuiteJsonCredentialsPath,
		gsuiteDomain:              opts.GsuiteDomain,

		reconcileLoopDuration: opts.ReconcileLoopDuration,
		syncedParentGroup:     opts.SyncedParentGroup,
	}

	gsuiteCli, err := gsuite.NewAdmin(context.Background(), runner.gsuiteJsonCredentialsPath)
	if err != nil {
		return nil, fmt.Errorf("failed creating gsuite client: %v", err)

	}

	keycloakObj, err := keycloak.NewKeycloak(keycloak.KeycloakOptions{
		AppCtx: opts.AppCtx,

		URI:          opts.KeycloakURI,
		Realm:        opts.KeycloakRealm,
		ClientID:     opts.KeycloakClientID,
		ClientSecret: opts.KeycloakClientSecret,
	})
	if err != nil {
		return nil, fmt.Errorf("failed creating keycloak client: %v", err)

	}

	runner.gsuiteCli = &gsuiteCli
	runner.keycloak = keycloakObj

	return runner, nil
}

// getKeycloakChildrenGroups TODO
func (r *Runner) getKeycloakChildrenGroups() (parentGroup *string, childrenGroups map[string]*gocloak.Group, err error) {

	// 1. Try retrieving Keycloak parent group
	kcExistingGroups, err := r.keycloak.GetGocloakClient().GetGroups(
		r.appCtx.Context, r.keycloak.GetToken().AccessToken, r.keycloak.Realm, gocloak.GetGroupsParams{
			Full:   gocloak.BoolP(true),
			Exact:  gocloak.BoolP(true),
			Max:    gocloak.IntP(1),
			Search: gocloak.StringP(r.syncedParentGroup),
		})
	if err != nil {
		return nil, nil, fmt.Errorf("failed getting parent group: %v", err)
	}

	// 2. Retrieve children groups for the found parent.
	// When the parent is not found, create it
	kcParentGroup := gocloak.Group{}
	kcChildrenGroups := []*gocloak.Group{}

	if len(kcExistingGroups) == 0 {
		kcParentGroup.Name = gocloak.StringP(r.syncedParentGroup)

		gCreationResult, err := r.keycloak.GetGocloakClient().CreateGroup(r.appCtx.Context,
			r.keycloak.GetToken().AccessToken, r.keycloak.Realm, kcParentGroup)

		if err != nil {
			return nil, nil, fmt.Errorf("failed creating parent group: %v", err)
		}

		kcParentGroup.ID = gocloak.StringP(gCreationResult)
	} else {
		kcParentGroup = *kcExistingGroups[0]
	}

	kcChildrenGroups, err = r.keycloak.GetChildrenGroups(r.keycloak.GetToken().AccessToken, *kcParentGroup.ID)
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

// getKeycloakUsersGroups return a map of username->{user, groups}
func (r *Runner) getKeycloakUsersGroups() (usersGroups map[string]KeycloakUserGroups, err error) {

	kcUsersGroups := map[string]KeycloakUserGroups{}

	kcUsers, err := r.keycloak.GetUsers(r.keycloak.GetToken().AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed getting users: %v", err)
	}

	// Create a map to merge a user and its groups into a unique object.
	for _, user := range kcUsers {

		kcUserGroups, err := r.keycloak.GetUserGroups(*user.ID, r.keycloak.GetToken().AccessToken)
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
			if !strings.HasPrefix(*kcUserGroup.Path, "/"+r.syncedParentGroup+"/") {
				continue
			}

			// Existing groups not present in Google
			if !slices.Contains(gsuiteGroups, *kcUserGroup.Name) {

				r.appCtx.Logger.Debug("deleting user from group", "user", kcUsername, "group", *kcUserGroup.Name)

				delUserGroupErr := r.keycloak.GetGocloakClient().DeleteUserFromGroup(r.appCtx.Context, r.keycloak.GetToken().AccessToken,
					r.keycloak.Realm, *kcUserGroups.User.ID, *kcChildrenGroups[*kcUserGroup.Name].ID)

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

				childGroupID, err := r.keycloak.GetGocloakClient().CreateChildGroup(r.appCtx.Context,
					r.keycloak.GetToken().AccessToken, r.keycloak.Realm, *kcParentGroupID, *tmpGroup)

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
			addUserGroupErr := r.keycloak.GetGocloakClient().AddUserToGroup(r.appCtx.Context, r.keycloak.GetToken().AccessToken,
				r.keycloak.Realm, *kcUserGroups.User.ID, *kcChildrenGroups[*tmpGroup.Name].ID)

			if addUserGroupErr != nil {
				r.appCtx.Logger.Error("failed adding user to the group",
					"user", kcUsername, "group", *tmpGroup.Name, "error", addUserGroupErr.Error())
			}
		}

	}
}

func (r *Runner) PleaseDoYourStuffForever() {
	for {
		// Renew Keycloak JWT
		err := r.keycloak.RenewToken()
		if err != nil {
			r.appCtx.Logger.Info("failed renewing Keycloak token", "error", err.Error())
			goto takeANap
		}

		//
		r.reconcileUserGroups()

	takeANap:
		r.appCtx.Logger.Info(fmt.Sprintf("reconcile group finished. waiting for the next loop in %s", r.reconcileLoopDuration.String()))
		time.Sleep(r.reconcileLoopDuration)
	}
}
