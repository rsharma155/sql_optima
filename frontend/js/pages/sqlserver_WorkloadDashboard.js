// SQL Optima — https://github.com/rsharma155/sql_optima
//
// Purpose: View adapter for SQL Server Workload Dashboard.
//          Standardizes initialization and route handling.
//
// Author: Ravi Sharma
// Copyright (c) 2026 Ravi Sharma
// SPDX-License-Identifier: MIT

import { ViewLoader } from '../modules/view-loader.js';
import { sqlserverWorkload } from '../modules/sqlserver_workload.js';

export const SqlServerWorkloadDashboardView = async () => {
    await ViewLoader.load('pages/sqlserver_workload.html', async () => {
        await sqlserverWorkload.init();
        return sqlserverWorkload;
    });
};

// Export to window for legacy router and global time-range refresh
window.SqlServerWorkloadDashboardView = SqlServerWorkloadDashboardView;
window.sqlserverWorkload = sqlserverWorkload;
