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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	//
	"github.com/Nerzal/gocloak/v13"
)

type Keycloak struct{}

// GetChildrenGroups return all the children groups for a specific group ID following pagination until the end.
func GetChildrenGroups(ctx context.Context, keycloakURL, realm, groupID, accessToken string) ([]*gocloak.Group, error) {
	var allGroups []*gocloak.Group
	paramFirst := 0
	paramMax := 100

	for {
		u := fmt.Sprintf("%s/admin/realms/%s/groups/%s/children?first=%d&max=%d",
			keycloakURL, realm, groupID, paramFirst, paramMax)

		//
		req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
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
