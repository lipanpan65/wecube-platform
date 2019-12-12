package com.webank.wecube.platform.core.service.user;

import com.webank.wecube.platform.core.commons.WecubeCoreException;
import com.webank.wecube.platform.core.domain.MenuItem;
import com.webank.wecube.platform.core.domain.RoleMenu;
import com.webank.wecube.platform.core.domain.plugin.PluginPackageMenu;
import com.webank.wecube.platform.core.dto.MenuItemDto;
import com.webank.wecube.platform.core.dto.user.RoleMenuDto;
import com.webank.wecube.platform.core.jpa.MenuItemRepository;
import com.webank.wecube.platform.core.jpa.PluginPackageMenuRepository;
import com.webank.wecube.platform.core.jpa.user.RoleMenuRepository;
import com.webank.wecube.platform.core.service.MenuService;
import com.webank.wecube.platform.core.service.datamodel.ExpressionServiceImpl;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Autowired;
import org.springframework.stereotype.Service;
import org.springframework.transaction.annotation.Transactional;

import java.util.ArrayList;
import java.util.List;
import java.util.Optional;
import java.util.stream.Collectors;

/**
 * @author howechen
 */
@Service
@Transactional(rollbackFor = Exception.class)
public class RoleMenuServiceImpl implements RoleMenuService {

    private static final Logger logger = LoggerFactory.getLogger(RoleMenuServiceImpl.class);
    private RoleMenuRepository roleMenuRepository;
    private MenuItemRepository menuItemRepository;
    private PluginPackageMenuRepository pluginPackageMenuRepository;

    @Autowired
    public RoleMenuServiceImpl(RoleMenuRepository roleMenuRepository,
                               MenuItemRepository menuItemRepository,
                               PluginPackageMenuRepository pluginPackageMenuRepository) {
        this.roleMenuRepository = roleMenuRepository;
        this.menuItemRepository = menuItemRepository;
        this.pluginPackageMenuRepository = pluginPackageMenuRepository;
    }

    /**
     * Retrieve role_menu table by given roleId
     *
     * @param roleId the id of role
     * @return role2MenuDto
     */
    @Override
    public RoleMenuDto retrieveMenusByRoleId(Long roleId) {
        List<RoleMenu> roleMenuList = this.roleMenuRepository.findAllByRoleId(roleId);
        List<MenuItemDto> menuCodeList = new ArrayList<>();
        roleMenuList.forEach(roleMenu -> {
            String menuCode = roleMenu.getMenuCode();
            // sys menu
            MenuItem sysMenu = this.menuItemRepository.findByCode(menuCode);
            // use {if sysMenu is null} to judge if this is a sys menu or package menu
            if (null != sysMenu) {
                menuCodeList.add(MenuItemDto.fromSystemMenuItem(sysMenu));
            } else {
                // package menu
                Optional<List<PluginPackageMenu>> allActivatePackageMenuByCode = this.pluginPackageMenuRepository.findAllActivateMenuByCode(menuCode);
                allActivatePackageMenuByCode.ifPresent(pluginPackageMenus -> pluginPackageMenus.forEach(pluginPackageMenu -> {
                    menuCodeList.add(this.transferPackageMenuToMenuItemDto(pluginPackageMenu));
                }));
            }

        });
        return new RoleMenuDto(roleId, menuCodeList);
    }

    /**
     * Update role_menu table
     *
     * @param roleId       given roleId
     * @param menuCodeList given total amount of the menuCode list
     * @return role2MenuDto
     */
    @Override
    public RoleMenuDto updateRoleToMenusByRoleId(Long roleId, List<String> menuCodeList) {
        List<RoleMenu> roleMenuList = this.roleMenuRepository.findAllByRoleId(roleId);

        // current menuCodeList - new menuCodeList = needToDeleteList
        List<RoleMenu> needToDeleteList = roleMenuList.stream().filter(roleMenu -> {
            String code = roleMenu.getMenuCode();
            return !menuCodeList.contains(code);
        }).collect(Collectors.toList());
        if (!needToDeleteList.isEmpty()) {
            for (RoleMenu roleMenu : needToDeleteList) {
                this.roleMenuRepository.deleteById(roleMenu.getId());
            }
        }

        // new menuCodeList - current menuCodeList = needToCreateList
        List<String> needToCreateList;
        List<String> currentMenuCodeList = roleMenuList.stream().map(RoleMenu::getMenuCode).collect(Collectors.toList());
        needToCreateList = menuCodeList.stream().filter(menuCode -> !currentMenuCodeList.contains(menuCode)).collect(Collectors.toList());

        if (!needToCreateList.isEmpty()) {
            List<RoleMenu> batchUpdateList = new ArrayList<>();
            needToCreateList.forEach(menuCode -> batchUpdateList.add(new RoleMenu(roleId, menuCode)));
            this.roleMenuRepository.saveAll(batchUpdateList);
        }

        return retrieveMenusByRoleId(roleId);
    }

    private MenuItemDto transferPackageMenuToMenuItemDto(PluginPackageMenu packageMenu) throws WecubeCoreException {
        MenuItem menuItem = menuItemRepository.findByCode(packageMenu.getCategory());
        if (null == menuItem) {
            String msg = String.format("Cannot find system menu item by package menu's category: [%s]",
                    packageMenu.getCategory());
            logger.error(msg);
            throw new WecubeCoreException(msg);
        }
        return MenuItemDto.fromPackageMenuItem(packageMenu, menuItem);
    }
}
