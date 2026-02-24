package usermgmt

import (
	"context"
	"fmt"
	"time"

	configstoreTables "github.com/maximhq/bifrost/framework/configstore/tables"
)

// startSyncWorker starts the background worker that syncs Keycloak orgs/groups to Bifrost.
func (p *UserMgmtPlugin) startSyncWorker() {
	interval := time.Duration(p.config.SyncIntervalSeconds) * time.Second
	if interval < 30*time.Second {
		interval = 30 * time.Second
	}

	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.logger.Info(fmt.Sprintf("[%s] Sync worker started (interval=%s)", PluginName, interval))

		// Initial sync
		p.syncKeycloakEntities()

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.ctx.Done():
				p.logger.Info(fmt.Sprintf("[%s] Sync worker stopped", PluginName))
				return
			case <-ticker.C:
				p.syncKeycloakEntities()
			}
		}
	}()
}

// syncKeycloakEntities syncs organizations and groups from Keycloak to Bifrost governance.
func (p *UserMgmtPlugin) syncKeycloakEntities() {
	ctx, cancel := context.WithTimeout(p.ctx, 60*time.Second)
	defer cancel()

	p.syncOrganizations(ctx)
	p.syncGroups(ctx)
}

// syncOrganizations syncs Keycloak organizations to governance customers.
func (p *UserMgmtPlugin) syncOrganizations(ctx context.Context) {
	if p.kcClient == nil || p.govStore == nil {
		return
	}

	orgs, err := p.kcClient.ListOrganizations(ctx)
	if err != nil {
		p.logger.Warn(fmt.Sprintf("[%s] Failed to list KC organizations: %v", PluginName, err))
		return
	}

	for _, org := range orgs {
		if !org.Enabled {
			continue
		}

		govData := p.govStore.GetGovernanceData()
		if _, exists := govData.Customers[org.ID]; !exists {
			customer := &configstoreTables.TableCustomer{
				ID:       org.ID,
				Name:     org.Name,
				IsActive: true,
			}
			p.govStore.CreateCustomerInMemory(customer)
			p.logger.Info(fmt.Sprintf("[%s] Synced KC org -> customer: %s (%s)", PluginName, org.Name, org.ID))
		}
	}
}

// syncGroups syncs Keycloak groups to governance teams.
func (p *UserMgmtPlugin) syncGroups(ctx context.Context) {
	if p.kcClient == nil || p.govStore == nil {
		return
	}

	groups, err := p.kcClient.ListGroups(ctx)
	if err != nil {
		p.logger.Warn(fmt.Sprintf("[%s] Failed to list KC groups: %v", PluginName, err))
		return
	}

	p.syncGroupsRecursive(groups)
}

// syncGroupsRecursive syncs groups and their subgroups.
func (p *UserMgmtPlugin) syncGroupsRecursive(groups []KCGroup) {
	for _, group := range groups {
		govData := p.govStore.GetGovernanceData()
		if _, exists := govData.Teams[group.ID]; !exists {
			team := &configstoreTables.TableTeam{
				ID:       group.ID,
				Name:     group.Name,
				IsActive: true,
			}
			p.govStore.CreateTeamInMemory(team)
			p.logger.Info(fmt.Sprintf("[%s] Synced KC group -> team: %s (%s)", PluginName, group.Name, group.ID))
		}

		if len(group.SubGroups) > 0 {
			p.syncGroupsRecursive(group.SubGroups)
		}
	}
}
