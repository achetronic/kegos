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

package keycloak

import (
	"encoding/json"
	"fmt"
	"io"
	"kegos/internal/globals"
	"net/http"
	//
	"github.com/Nerzal/gocloak/v13"
)

type KeycloakOptions struct {
	AppCtx *globals.ApplicationContext

	URI          string
	Realm        string
	ClientID     string
	ClientSecret string
}

type Keycloak struct {
	appCtx *globals.ApplicationContext

	URI          string
	Realm        string
	ClientID     string
	ClientSecret string

	gocloakCli         *gocloak.GoCloak
	gocloakAccessToken *gocloak.JWT
}

func NewKeycloak(opts KeycloakOptions) (*Keycloak, error) {

	object := &Keycloak{
		appCtx: opts.AppCtx,

		URI:          opts.URI,
		Realm:        opts.Realm,
		ClientID:     opts.ClientID,
		ClientSecret: opts.ClientSecret,
	}

	gcClient := gocloak.NewClient(object.URI)
	object.gocloakCli = gcClient

	return object, nil
}

// RenewToken renew JWTs in Keycloak server and store it into Keycloak object
func (k *Keycloak) RenewToken() error {
	tmpToken, err := k.gocloakCli.LoginClient(k.appCtx.Context, k.ClientID, k.ClientSecret, k.Realm)
	if err != nil {
		return fmt.Errorf("failed signing in: %s", err.Error())
	}

	k.gocloakAccessToken = tmpToken
	return nil
}

// GetToken ...
func (k *Keycloak) GetToken() *gocloak.JWT {
	return k.gocloakAccessToken
}

// GetGocloakClient ...
func (k *Keycloak) GetGocloakClient() *gocloak.GoCloak {
	return k.gocloakCli
}

// GetGroups return all the groups following pagination until the end.
func (k *Keycloak) GetGroups(accessToken string) ([]*gocloak.Group, error) {
	var allGroups []*gocloak.Group
	paramFirst := 0
	paramMax := 100

	for {

		tmpGroups, err := k.gocloakCli.GetGroups(k.appCtx.Context, accessToken, k.Realm, gocloak.GetGroupsParams{
			First: gocloak.IntP(paramFirst),
			Max:   gocloak.IntP(paramMax),
		})
		if err != nil {
			return nil, fmt.Errorf("failed getting groups: %v", err)
		}

		allGroups = append(allGroups, tmpGroups...)

		// When we receive fewer than max, there are no more pages
		if len(tmpGroups) < paramMax {
			break
		}

		paramFirst += paramMax
	}

	return allGroups, nil
}

// GetChildrenGroups return all the children groups for a specific group ID following pagination until the end.
func (k *Keycloak) GetChildrenGroups(accessToken, groupID string) ([]*gocloak.Group, error) {
	var allGroups []*gocloak.Group
	paramFirst := 0
	paramMax := 100

	for {
		u := fmt.Sprintf("%s/admin/realms/%s/groups/%s/children?first=%d&max=%d",
			k.URI, k.Realm, groupID, paramFirst, paramMax)

		//
		req, err := http.NewRequestWithContext(k.appCtx.Context, "GET", u, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		//
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json")

		// Perform the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to make request: %w", err)
		}
		defer resp.Body.Close()

		// Verify response
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
		}

		//
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}

		var groups []*gocloak.Group
		if err := json.Unmarshal(body, &groups); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}

		allGroups = append(allGroups, groups...)

		// When we receive fewer than max, there are no more pages
		if len(groups) < paramMax {
			break
		}

		paramFirst += paramMax
	}

	return allGroups, nil
}

// GetUsers return all the children users following pagination until the end.
func (k *Keycloak) GetUsers(accessToken string) ([]*gocloak.User, error) {

	var allUsers []*gocloak.User
	paramFirst := 0
	paramMax := 100

	for {
		tmpUsers, err := k.gocloakCli.GetUsers(k.appCtx.Context, accessToken, k.Realm, gocloak.GetUsersParams{
			First: gocloak.IntP(paramFirst),
			Max:   gocloak.IntP(paramMax),
		})
		if err != nil {
			return nil, fmt.Errorf("failed getting users: %v", err)
		}

		allUsers = append(allUsers, tmpUsers...)

		// When we receive fewer than max, there are no more pages
		if len(tmpUsers) < paramMax {
			break
		}

		paramFirst += paramMax
	}

	return allUsers, nil
}

// GetUserGroups return all the groups attached to a user following pagination until the end.
func (k *Keycloak) GetUserGroups(userID, accessToken string) ([]*gocloak.Group, error) {

	var allGroups []*gocloak.Group
	paramFirst := 0
	paramMax := 100

	for {
		tmpGroups, err := k.gocloakCli.GetUserGroups(k.appCtx.Context, accessToken, k.Realm, userID, gocloak.GetGroupsParams{
			First: gocloak.IntP(paramFirst),
			Max:   gocloak.IntP(paramMax),
		})
		if err != nil {
			return nil, fmt.Errorf("failed getting user groups: %v", err)
		}

		allGroups = append(allGroups, tmpGroups...)

		// When we receive fewer than max, there are no more pages
		if len(tmpGroups) < paramMax {
			break
		}

		paramFirst += paramMax
	}

	return allGroups, nil
}
