// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

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

// gsuiteClient is the subset of the Gsuite admin API the runner depends on.
type gsuiteClient interface {
	GetGroupsFromUser(domain string, user string) (groups []string, err error)
}

type RunnerOptions struct {
	AppCtx *globals.ApplicationContext

	GsuiteJsonCredentialsPath string
	GsuiteDomains             []string
	UserRateLimit             int

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
	gsuiteDomains             []string
	userDelay                 time.Duration

	//
	reconcileLoopDuration time.Duration
	syncedParentGroup     string

	//
	gsuiteCli gsuiteClient
	keycloak  *keycloak.Keycloak
}

func NewRunner(opts RunnerOptions) (*Runner, error) {

	runner := &Runner{
		appCtx:                    opts.AppCtx,
		gsuiteJsonCredentialsPath: opts.GsuiteJsonCredentialsPath,
		gsuiteDomains:             opts.GsuiteDomains,
		userDelay:                 userDelayFromRate(opts.UserRateLimit),

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

// userDelayFromRate converts a users-per-minute rate into the pause between users.
// A rate of zero or below disables throttling.
func userDelayFromRate(usersPerMinute int) time.Duration {
	if usersPerMinute <= 0 {
		return 0
	}
	return time.Minute / time.Duration(usersPerMinute)
}

// getGsuiteGroupsForUser returns the union of the user's Gsuite groups across every configured
// domain, deduplicated. A user's login email is passed as userKey directly; Google accepts either
// the primary email or an alias, so no alias resolution is needed. The domain filter selects the
// domain where the groups themselves live, which is an account-level setting rather than a per-user
// property (e.g. groups may live under one domain while users log in through another).
func (r *Runner) getGsuiteGroupsForUser(username string) (groups []string, err error) {
	seen := map[string]struct{}{}

	for _, domain := range r.gsuiteDomains {
		domainGroups, err := r.gsuiteCli.GetGroupsFromUser(domain, username)
		if err != nil {
			return nil, fmt.Errorf("failed getting groups for %s in domain %s: %v", username, domain, err)
		}

		for _, group := range domainGroups {
			if _, found := seen[group]; found {
				continue
			}
			seen[group] = struct{}{}
			groups = append(groups, group)
		}
	}

	return groups, nil
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

		if r.userDelay > 0 {
			time.Sleep(r.userDelay)
		}

		r.appCtx.Logger.Info("reconciling user groups", "user", kcUsername)

		gsuiteGroups, err := r.getGsuiteGroupsForUser(kcUsername)
		if err != nil {
			r.appCtx.Logger.Error("failed getting groups from Gsuite. Ignoring user...", "user", kcUsername, "error", err.Error())
			continue
		}

		if len(gsuiteGroups) == 0 {
			r.appCtx.Logger.Debug("user has no groups in any configured domain", "user", kcUsername)
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
