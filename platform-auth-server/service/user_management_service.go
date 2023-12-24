package service

import (
	"errors"
	"fmt"
	"github.com/WeBankPartners/wecube-platform/platform-auth-server/common/constant"
	"github.com/WeBankPartners/wecube-platform/platform-auth-server/common/exterror"
	"github.com/WeBankPartners/wecube-platform/platform-auth-server/common/log"
	"github.com/WeBankPartners/wecube-platform/platform-auth-server/common/utils"
	"github.com/WeBankPartners/wecube-platform/platform-auth-server/model"
	"github.com/WeBankPartners/wecube-platform/platform-auth-server/service/db"
	"golang.org/x/crypto/bcrypt"
	"time"
)

const DefPasswordLength = 6

var UserManagementServiceInstance UserManagementService

type UserManagementService struct {
}

func (UserManagementService) ResetLocalUserPassword(userPassDto model.SimpleLocalUserPassDto) (string, error) {
	username := userPassDto.Username
	if len(username) == 0 {
		return "", exterror.NewAuthServerError("Username cannot be blank.")
	}

	encoedPwd, err := doResetLocalUserPassword(username)
	return encoedPwd, err
}

func doResetLocalUserPassword(username string) (string, error) {
	user, err := db.UserRepositoryInstance.FindNotDeletedUserByUsername(username)
	if err != nil {
		return "", err
	}
	if user == nil {
		log.Logger.Debug(fmt.Sprintf("Such user does not exist with username %v", username))
		// msg := fmt.Sprintf("Failed to modify a none existed user with username {%s}.", username)
		return "", exterror.Catch(exterror.New().AuthServer3021Error, nil)
	}

	if utils.EqualsIgnoreCase(constant.AuthSourceUm, user.AuthSource) {
		return "", exterror.NewAuthServerError("Cannot modify password of UM user account.")
	}

	ranPassword := PasswordGeneratorInstance.RandomPassword(DefPasswordLength)

	encodedNewPassword, err := encodePassword(ranPassword)
	if err != nil {
		return "", err
	}
	user.Password = encodedNewPassword

	if cnt, err := db.Engine.Insert(user); cnt == 0 || err != nil {
		if err != nil {
			log.Logger.Error("failed to insert user", log.Error(err))
		}
		return "", errors.New("failed to inser user")
	}

	return ranPassword, nil
}

func encodePassword(rawPassword string) (string, error) {
	if len(rawPassword) == 0 {
		return "", nil
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(rawPassword), 14)
	if err != nil {
		return "", err
	}
	return string(hashedPassword), nil
}

func (UserManagementService) ModifyLocalUserPassword(userPassDto *model.SimpleLocalUserPassDto) (*model.SimpleLocalUserDto, error) {
	username := userPassDto.Username
	if len(username) == 0 {
		return nil, exterror.NewAuthServerError("Username cannot be blank.")
	}

	originalPassword := userPassDto.OriginalPassword
	toChangePassword := userPassDto.ChangedPassword

	if len(originalPassword) == 0 || len(toChangePassword) == 0 {
		return nil, exterror.NewAuthServerError("Password cannot be blank.")
	}

	return doModifyLocalUserPassword(username, originalPassword, toChangePassword)
}

func doModifyLocalUserPassword(username, originalPassword, toChangePassword string) (*model.SimpleLocalUserDto, error) {
	user, err := db.UserRepositoryInstance.FindNotDeletedUserByUsername(username)
	if err != nil {
		return nil, err
	}
	if user == nil {
		log.Logger.Debug(fmt.Sprintf("Such user does not exist with username %s", username))
		//msg := fmt.Sprintf("Failed to modify a none existed user with username {%s}.", username)
		return nil, exterror.Catch(exterror.New().AuthServer3021Error.WithParam(username), nil)
	}

	if utils.EqualsIgnoreCase(constant.AuthSourceUm, user.AuthSource) {
		return nil, exterror.NewAuthServerError("Cannot modify password of UM user account.")
	}

	if len(user.Password) == 0 {
		return nil, exterror.NewAuthServerError("The password of user to modify is blank.")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(originalPassword)); err != nil {
		return nil, exterror.NewAuthServerError("The password of user to modify is invalid.")
	}

	encodedNewPassword, err := encodePassword(toChangePassword)
	if err != nil {
		return nil, err
	}
	user.Password = encodedNewPassword

	if affected, err := db.Engine.Update(user); affected == 0 || err != nil {
		if err != nil {
			log.Logger.Error(fmt.Sprintf("failed to update user:%v", user.Id), log.Error(err))
		}
		return nil, errors.New("failed to update user")
	}

	return convertToSimpleLocalUserDto(user), nil

}

func convertToSimpleLocalUserDto(user *model.SysUserEntity) *model.SimpleLocalUserDto {
	return &model.SimpleLocalUserDto{
		ID:          user.Id,
		Username:    user.Username,
		Password:    "",
		Department:  user.Department,
		EmailAddr:   user.EmailAddr,
		Title:       user.Title,
		EnglishName: user.EnglishName,
		NativeName:  user.LocalName,
		CellPhoneNo: user.CellPhoneNo,
		OfficeTelNo: user.OfficeTelNo,
		Active:      user.IsActive,
		Blocked:     user.IsBlocked,
	}
}

//@Transactional
func (UserManagementService) RevokeRoleFromUsers(roleId string, userDtos []*model.SimpleLocalUserDto, curUser string) error {
	session := db.Engine.NewSession()
	session.Begin()
	defer session.Close()

	var role *model.SysRoleEntity
	_, err := session.ID(roleId).Get(&role)
	if err != nil {
		session.Rollback()
		return err
	}

	if role == nil {
		session.Rollback()
		log.Logger.Debug(fmt.Sprintf("revoking user roles error:such role entity does not exist, role id %v", roleId))
		return exterror.Catch(exterror.New().AuthServer3018Error.WithParam(roleId), nil)
	}

	for _, userDto := range userDtos {
		userRole, err := db.UserRoleRsRepositoryInstance.FindOneByUserIdAndRoleId(userDto.ID, role.Id, session)
		if err != nil {
			session.Rollback()
			return err
		}
		if userRole == nil {
			continue
		}

		if userRole.Deleted {
			continue
		}

		userRole.Deleted = true
		userRole.UpdatedBy = curUser
		userRole.UpdatedTime = time.Now()
		if affected, err := session.Update(userRole); affected == 0 || err != nil {
			if err != nil {
				log.Logger.Error(fmt.Sprintf("failed to update userRoleRs:%v", userRole.Id), log.Error(err))
			}
			session.Rollback()
			return errors.New(fmt.Sprintf("failed to update userRoleRs:%v", userRole.Id))
		}
	}
	session.Commit()
	return nil
}

//@Transactional
func (UserManagementService) ConfigureUserWithRoles(userId string, roleDtos []*model.SimpleLocalRoleDto, curUser string) error {
	session := db.Engine.NewSession()
	session.Begin()
	defer session.Close()

	if len(roleDtos) == 0 {
		return nil
	}
	var user *model.SysUserEntity

	if _, err := session.ID(userId).Get(&user); err != nil {
		log.Logger.Error(fmt.Sprintf("failed to get user:%v", userId), log.Error(err))
	}
	if user == nil {
		session.Rollback()
		return exterror.Catch(exterror.New().AuthServer3024Error.WithParam(userId), nil)
	}

	existUserRoles, err := db.UserRoleRsRepositoryInstance.FindAllByUserId(user.Id)
	if err != nil {
		session.Rollback()
		return err
	}

	if existUserRoles == nil {
		existUserRoles = make([]*model.UserRoleRsEntity, 0)
	}

	remainUserRoleMap := make(map[string]string)
	remainUserRoles := make([]*model.UserRoleRsEntity, 0)

	for _, roleDto := range roleDtos {
		var role *model.SysRoleEntity
		if _, err := session.ID(roleDto.ID).Get(&role); err != nil {
			session.Rollback()
			return err
		}
		if role == nil {
			session.Rollback()
			return exterror.Catch(exterror.New().AuthServer3012Error, nil)
		}

		if role.Deleted {
			continue
		}
		foundUserRoles := findFromUserRoles(existUserRoles, role.Id)
		remainUserRoles = append(remainUserRoles, foundUserRoles...)
		for _, userRole := range foundUserRoles {
			remainUserRoleMap[userRole.Id] = "1"
		}

		userRole, err := db.UserRoleRsRepositoryInstance.FindOneByUserIdAndRoleId(userId, role.Id, session)
		if err != nil {
			session.Rollback()
			return err
		}
		if userRole != nil {
			log.Logger.Info(fmt.Sprintf("such user userRole configuration already exist,userId=%s,roleId=%a", userId, role.Id))
			continue
		} else {
			userRole := &model.UserRoleRsEntity{
				CreatedBy: curUser,
				UserId:    userId,
				Username:  user.Username,
				RoleId:    role.Id,
				RoleName:  role.Name,
			}
			affected, err := session.Insert(userRole)
			if err != nil || affected == 0 {
				if err != nil {
					log.Logger.Error("failed to insert userRole", log.Error(err))
				}
				session.Rollback()
				return errors.New("failed to insert userRole")
			}
		}

	}

	resultRoles := make([]*model.UserRoleRsEntity, 0)
	for _, userRole := range existUserRoles {
		if _, ok := remainUserRoleMap[userRole.Id]; !ok {
			resultRoles = append(resultRoles, userRole)
		}
	}
	//existUserRoles.removeAll(remainUserRoles);

	for _, ur := range resultRoles {
		ur.Active = false
		ur.Deleted = true
		ur.UpdatedBy = curUser
		ur.UpdatedTime = time.Now()

		affected, err := session.Update(ur)
		if affected == 0 || err != nil {
			if err != nil {
				log.Logger.Error("failed to update userRole", log.Error(err))
			}
			session.Rollback()
			return errors.New("failed to update userRole")
		}
	}

	session.Commit()
	return nil
}

func findFromUserRoles(existUserRoles []*model.UserRoleRsEntity, roleId string) []*model.UserRoleRsEntity {
	remainUserRoles := make([]*model.UserRoleRsEntity, 0)
	for _, ur := range existUserRoles {
		if roleId == ur.RoleId {
			remainUserRoles = append(remainUserRoles, ur)
		}
	}

	return remainUserRoles
}

//@Transactional
func (UserManagementService) RevokeRolesFromUser(userId string, roleDtos []*model.SimpleLocalRoleDto, curUser string) error {
	session := db.Engine.NewSession()
	session.Begin()
	defer session.Close()

	if len(roleDtos) == 0 {
		session.Rollback()
		return nil
	}
	var user *model.SysUserEntity
	_, err := session.ID(userId).Get(&user)
	if err != nil {
		session.Rollback()
		return err
	}

	if user == nil {
		session.Rollback()
		return exterror.Catch(exterror.New().AuthServer3024Error.WithParam(userId), nil)
	}

	for _, roleDto := range roleDtos {
		userRole, err := db.UserRoleRsRepositoryInstance.FindOneByUserIdAndRoleId(user.Id, roleDto.ID, session)
		if err != nil {
			session.Rollback()
			return err
		}
		if userRole == nil {
			continue
		}

		if userRole.Deleted {
			continue
		}

		userRole.Deleted = true
		userRole.UpdatedBy = curUser
		userRole.UpdatedTime = time.Now()
		affected, err := session.Update(userRole)
		if affected == 0 || err != nil {
			if err != nil {
				log.Logger.Error("failed to update userRole", log.Error(err))
			}
			session.Rollback()
			return errors.New(fmt.Sprintf("failed to update userRole:%s", userRole.Id))
		}
	}
	session.Commit()
	return nil
}

//@Transactional
func (UserManagementService) ConfigureRoleForUsers(roleId string, userDtos []*model.SimpleLocalUserDto, curUser string) error {
	session := db.Engine.NewSession()
	session.Begin()
	defer session.Close()

	var role *model.SysRoleEntity
	_, err := session.ID(roleId).Get(&role)
	if err != nil {
		session.Rollback()
		return err
	}

	if role == nil {
		session.Rollback()
		return exterror.Catch(exterror.New().AuthServer3012Error.WithParam(roleId), nil)
	}

	if role.Deleted {
		session.Rollback()
		return nil
	}

	for _, userDto := range userDtos {

		var user *model.SysUserEntity

		if _, err := session.ID(userDto.ID).Get(&user); err != nil {
			log.Logger.Error(fmt.Sprintf("failed to get user:%v", userDto.ID), log.Error(err))
		}
		if user == nil {
			session.Rollback()
			return exterror.Catch(exterror.New().AuthServer3019Error.WithParam(userDto.ID), nil)
		}

		userRole, err := db.UserRoleRsRepositoryInstance.FindOneByUserIdAndRoleId(userDto.ID, roleId, session)
		if err != nil {
			session.Rollback()
			return err
		}
		if userRole != nil {
			log.Logger.Info(fmt.Sprintf("such user role configuration already exist,userId=%s,roleId=%s", userDto.ID, roleId))
			continue
		} else {
			userRole = &model.UserRoleRsEntity{
				CreatedBy: curUser,
				UserId:    userDto.ID,
				Username:  user.Username,
				RoleId:    roleId,
				RoleName:  role.Name,
			}

			affected, err := session.Insert(userRole)
			if err != nil || affected == 0 {
				if err != nil {
					log.Logger.Error("failed to insert userRole", log.Error(err))
				}
				session.Rollback()
				return errors.New("failed to insert userRole")
			}

		}

	}
	session.Commit()
	return nil
}
