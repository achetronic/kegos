// SPDX-FileCopyrightText: 2026 Alby Hernández <hola@achetronic.com>
// SPDX-License-Identifier: Apache-2.0

package gsuite

import (
	"log"
	"os"

	//
	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"
)

const UnableGetGroupMembersErrorMessage = "unable to get group members: %s"

type Admin struct {
	Ctx context.Context

	//
	service      *admin.Service
	tokenSource  oauth2.TokenSource
	jsonFilepath string
}

type GroupMembers struct {
	Group string
	Users []string
}

func NewAdmin(ctx context.Context, googleJsonFilepath string) (adminObj Admin, err error) {
	adminObj.Ctx = ctx
	adminObj.jsonFilepath = googleJsonFilepath

	err = adminObj.getAdminTokenSource()
	if err != nil {
		return adminObj, err
	}

	adminObj.service, err = admin.NewService(ctx, option.WithTokenSource(adminObj.tokenSource))

	return adminObj, err
}

// getAdminTokenSource TODO
func (a *Admin) getAdminTokenSource() (err error) {

	jsonCredentials, err := os.ReadFile(a.jsonFilepath)
	if err != nil {
		return err
	}

	config, err := google.JWTConfigFromJSON(jsonCredentials,
		admin.AdminDirectoryGroupReadonlyScope,
		admin.AdminDirectoryUserReadonlyScope)
	if err != nil {
		return err
	}

	a.tokenSource = config.TokenSource(a.Ctx)

	//tokenSource, err := google.DefaultTokenSource(ctx)
	//if err != nil {
	//	log.Fatal(err)
	//}
	return err
}

func (a *Admin) GetAllGroups(domain string) (groups []string, err error) {

	err = a.service.Groups.
		List().
		Domain(domain).
		Pages(a.Ctx, func(adGroups *admin.Groups) error {
			for _, group := range adGroups.Groups {
				groups = append(groups, group.Email)
			}
			return nil
		})

	return groups, err
}

// GetAllUsers me das un dominio y te devuelvo la lista de usuarios completa
func (a *Admin) GetAllUsers(domain string) (users []string, err error) {

	err = a.service.Users.
		List().
		Domain(domain).
		Pages(a.Ctx, func(adUsers *admin.Users) error {
			for _, user := range adUsers.Users {
				users = append(users, user.PrimaryEmail)
			}
			return nil
		})

	return users, err
}

// GetGroupsFromUser me das un usuario y te doy todos los grupos del usuario
func (a *Admin) GetGroupsFromUser(domain string, user string) (groups []string, err error) {
	err = a.service.Groups.
		List().
		Domain(domain).
		UserKey(user).
		Pages(a.Ctx, func(groupsReport *admin.Groups) error {
			for _, m := range groupsReport.Groups {
				groups = append(groups, m.Email)
			}
			return nil
		})

	return groups, err
}

// GetUserPrimaryEmail resolves any user key (primary email or alias) to the account's primary email.
func (a *Admin) GetUserPrimaryEmail(userKey string) (primaryEmail string, err error) {
	user, err := a.service.Users.Get(userKey).Do()
	if err != nil {
		return "", err
	}
	return user.PrimaryEmail, nil
}

// GetUsersFromGroup me das un grupo y te devuelvo sus miembros
func (a *Admin) GetUsersFromGroup(group string) (memberList []string, err error) {

	err = a.service.Members.
		List(group).
		Pages(a.Ctx, func(adMembers *admin.Members) error {
			for _, member := range adMembers.Members {
				memberList = append(memberList, member.Email)
			}
			return nil
		})

	return memberList, err
}

// GetGroupsMembers Me das una lista de grupos y te devuelvo una lista de grupos con sus miembros dentro
// Ref: https://developers.google.com/admin-sdk/directory/reference/rest/v1/members/list
func (a *Admin) GetGroupsMembers(groups []string) (groupsMembers []GroupMembers, err error) {

	for _, group := range groups {
		users, err := a.GetUsersFromGroup(group)
		if err != nil {
			log.Printf(UnableGetGroupMembersErrorMessage, err.Error())
			continue
		}
		groupsMembers = append(groupsMembers, GroupMembers{Group: group, Users: users})
	}

	return groupsMembers, err
}
