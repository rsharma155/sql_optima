/*
 * SQL Optima — https://github.com/rsharma155/sql_optima
 *
 * Purpose: Cloud Native PostgreSQL cluster topology visualization component.
 *
 * Author: Ravi Sharma
 * Copyright (c) 2026 Ravi Sharma
 * SPDX-License-Identifier: MIT
 */

import React, { useState, useEffect, useCallback } from 'react';

interface ReplicaStat {
  replica_pod_name: string;
  pod_ip: string;
  state: string;
  sync_state: string;
  replay_lag_mb: number;
}

interface ReplicationPayload {
  is_primary: boolean;
  local_lag_mb: number;
  cluster_state: string;
  max_lag_mb: number;
  wal_gen_rate_mbps: number;
  bg_writer_eff_pct: number;
  standbys: ReplicaStat[];
}

const POLL_INTERVAL_MS = 15000;
const LAG_CRITICAL_THRESHOLD = 50;

function syncStateColor(sync: string): string {
  if (sync === 'sync' || sync === 'quorum') return '#22c55e';
  if (sync === 'potential') return '#f59e0b';
  return '#6b7280';
}

function stateRowColor(state: string): string {
  if (state !== 'streaming') return '#f59e0b';
  return '';
}

function lagColor(lag: number): { color: string; fontWeight: string } {
  if (lag > LAG_CRITICAL_THRESHOLD) return { color: '#ef4444', fontWeight: 'bold' };
  return { color: '', fontWeight: '' };
}

function gaugeColor(lag: number): string {
  return lag > LAG_CRITICAL_THRESHOLD ? '#ef4444' : '#22c55e';
}

function formatLag(mb: number): string {
  return mb.toFixed(2);
}

export default function CNPGClusterTopology() {
  const [data, setData] = useState<ReplicationPayload | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastPoll, setLastPoll] = useState<Date | null>(null);

  const fetchReplication = useCallback(async () => {
    try {
      const inst = (window as any).appState?.config?.instances?.[(window as any).appState?.currentInstanceIdx];
      if (!inst || inst.type !== 'postgres') {
        setError('No PostgreSQL instance selected');
        setLoading(false);
        return;
      }

      const res = await (window as any).apiClient.authenticatedFetch(
        `/api/postgres/replication?instance=${encodeURIComponent(inst.name)}`
      );

      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }

      const contentType = res.headers.get('content-type') || '';
      if (!contentType.includes('application/json')) {
        const text = await res.text();
        console.error('Replication API returned non-JSON:', text.substring(0, 200));
        throw new Error('Invalid response from replication API');
      }

      const payload: ReplicationPayload = await res.json();
      setData(payload);
      setError(null);
      setLastPoll(new Date());
    } catch (e: any) {
      console.error('[CNPGClusterTopology] fetch error:', e);
      setError(e.message || 'Failed to fetch replication data');
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchReplication();
    const timer = setInterval(fetchReplication, POLL_INTERVAL_MS);
    return () => clearInterval(timer);
  }, [fetchReplication]);

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', padding: '3rem' }}>
        <div className="spinner" />
        <span style={{ marginLeft: '1rem', color: 'var(--text-muted)' }}>Polling CNPG cluster topology...</span>
      </div>
    );
  }

  if (error) {
    return (
      <div style={{ padding: '1rem', border: '1px solid var(--danger)', borderRadius: '8px', background: 'rgba(239,68,68,0.08)', color: 'var(--danger)' }}>
        <i className="fa-solid fa-exclamation-triangle" /> {error}
      </div>
    );
  }

  if (!data) return null;

  const isPrimary = data.is_primary;

  return (
    <div>
      {/* Role Badge */}
      <div style={{ marginBottom: '1.5rem' }}>
        {isPrimary ? (
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: '0.5rem',
            background: 'rgba(34,197,94,0.12)', border: '1px solid #22c55e',
            borderRadius: '6px', padding: '0.4rem 1rem', fontSize: '0.85rem',
            color: '#22c55e', fontWeight: 600,
          }}>
            <i className="fa-solid fa-crown" /> Role: Primary Node
          </span>
        ) : (
          <span style={{
            display: 'inline-flex', alignItems: 'center', gap: '0.5rem',
            background: 'rgba(59,130,246,0.12)', border: '1px solid #3b82f6',
            borderRadius: '6px', padding: '0.4rem 1rem', fontSize: '0.85rem',
            color: '#3b82f6', fontWeight: 600,
          }}>
            <i className="fa-solid fa-book-open" /> Role: Standby Node (Read-Only)
          </span>
        )}
        {lastPoll && (
          <span style={{ marginLeft: '1rem', fontSize: '0.75rem', color: 'var(--text-muted)' }}>
            Last poll: {lastPoll.toLocaleTimeString()}
          </span>
        )}
      </div>

      {/* Primary View: Replica Data Grid */}
      {isPrimary && (
        <div className="glass-panel" style={{ padding: '0.75rem' }}>
          <h3 style={{ fontSize: '0.85rem', margin: '0 0 0.75rem 0', color: 'var(--text)' }}>
            <i className="fa-solid fa-clone text-accent" /> Connected CNPG Replicas ({data.standbys.length})
          </h3>

          {data.standbys.length === 0 ? (
            <div style={{ textAlign: 'center', padding: '2rem', color: 'var(--text-muted)' }}>
              <i className="fa-solid fa-info-circle" /> No connected replicas found in pg_stat_replication
            </div>
          ) : (
            <div className="table-responsive" style={{ maxHeight: '400px', overflowY: 'auto' }}>
              <table className="data-table" style={{ fontSize: '0.75rem' }}>
                <thead>
                  <tr>
                    <th>Pod Name</th>
                    <th>IP</th>
                    <th>State</th>
                    <th>Sync Mode</th>
                    <th>Lag (MB)</th>
                  </tr>
                </thead>
                <tbody>
                  {data.standbys.map((replica, idx) => {
                    const rowBg = stateRowColor(replica.state);
                    const lagStyle = lagColor(replica.replay_lag_mb);
                    return (
                      <tr key={idx} style={rowBg ? { background: `${rowBg}15` } : {}}>
                        <td>
                          <strong style={{ fontFamily: 'monospace', fontSize: '0.8rem' }}>
                            {replica.replica_pod_name}
                          </strong>
                        </td>
                        <td style={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                          {replica.pod_ip || 'N/A'}
                        </td>
                        <td>
                          <span className={`badge ${replica.state === 'streaming' ? 'badge-success' : 'badge-warning'}`}>
                            {replica.state}
                          </span>
                        </td>
                        <td style={{ color: syncStateColor(replica.sync_state), fontWeight: 600 }}>
                          {replica.sync_state}
                        </td>
                        <td style={{ color: lagStyle.color, fontWeight: lagStyle.fontWeight }}>
                          {formatLag(replica.replay_lag_mb)}
                          {replica.replay_lag_mb > LAG_CRITICAL_THRESHOLD && (
                            <i className="fa-solid fa-exclamation-triangle" style={{ marginLeft: '4px', fontSize: '0.7rem' }} />
                          )}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}

          {/* Summary Stats */}
          <div style={{ display: 'flex', gap: '1rem', marginTop: '1rem', flexWrap: 'wrap' }}>
            <div style={{ flex: 1, minWidth: '120px', padding: '0.5rem', background: 'var(--bg-tertiary)', borderRadius: '6px', textAlign: 'center' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Max Lag</div>
              <div style={{ fontSize: '1.2rem', fontWeight: 700, color: gaugeColor(data.max_lag_mb) }}>
                {formatLag(data.max_lag_mb)} <span style={{ fontSize: '0.6em' }}>MB</span>
              </div>
            </div>
            <div style={{ flex: 1, minWidth: '120px', padding: '0.5rem', background: 'var(--bg-tertiary)', borderRadius: '6px', textAlign: 'center' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>BGWriter Eff</div>
              <div style={{ fontSize: '1.2rem', fontWeight: 700, color: 'var(--success)' }}>
                {data.bg_writer_eff_pct.toFixed(0)}<span style={{ fontSize: '0.6em' }}>%</span>
              </div>
            </div>
            <div style={{ flex: 1, minWidth: '120px', padding: '0.5rem', background: 'var(--bg-tertiary)', borderRadius: '6px', textAlign: 'center' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Replicas</div>
              <div style={{ fontSize: '1.2rem', fontWeight: 700, color: 'var(--accent-blue)' }}>
                {data.standbys.length}
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Standby View: Local Replay Lag Gauge */}
      {!isPrimary && (
        <div className="glass-panel" style={{ padding: '1.5rem', textAlign: 'center' }}>
          <h3 style={{ fontSize: '0.85rem', margin: '0 0 1.5rem 0', color: 'var(--text)' }}>
            <i className="fa-solid fa-clock text-accent" /> Local Replay Lag
          </h3>

          {/* Gauge Card */}
          <div style={{
            display: 'inline-flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center',
            width: '200px', height: '200px', borderRadius: '50%',
            border: `4px solid ${gaugeColor(data.local_lag_mb)}`,
            background: `${gaugeColor(data.local_lag_mb)}08`,
            position: 'relative',
          }}>
            <div style={{ fontSize: '2.5rem', fontWeight: 800, color: gaugeColor(data.local_lag_mb) }}>
              {formatLag(data.local_lag_mb)}
            </div>
            <div style={{ fontSize: '0.85rem', color: 'var(--text-muted)', marginTop: '0.25rem' }}>MB</div>
            {data.local_lag_mb > LAG_CRITICAL_THRESHOLD && (
              <div style={{
                position: 'absolute', top: '-10px', right: '-10px',
                background: '#ef4444', color: '#fff', borderRadius: '50%',
                width: '24px', height: '24px', display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: '0.75rem',
              }}>
                <i className="fa-solid fa-exclamation-triangle" />
              </div>
            )}
          </div>

          <div style={{ marginTop: '1.5rem', fontSize: '0.8rem', color: 'var(--text-muted)' }}>
            {data.local_lag_mb > LAG_CRITICAL_THRESHOLD ? (
              <span style={{ color: '#ef4444', fontWeight: 600 }}>
                <i className="fa-solid fa-exclamation-triangle" /> CRITICAL: Replay lag exceeds {LAG_CRITICAL_THRESHOLD} MB
              </span>
            ) : data.local_lag_mb > 10 ? (
              <span style={{ color: '#f59e0b', fontWeight: 600 }}>
                <i className="fa-solid fa-exclamation-circle" /> WARNING: Replay lag is elevated
              </span>
            ) : (
              <span style={{ color: '#22c55e' }}>
                <i className="fa-solid fa-check-circle" /> Healthy: Replica is nearly in sync
              </span>
            )}
          </div>

          {/* Summary Stats */}
          <div style={{ display: 'flex', gap: '1rem', marginTop: '1.5rem', flexWrap: 'wrap', justifyContent: 'center' }}>
            <div style={{ flex: 1, minWidth: '120px', padding: '0.5rem', background: 'var(--bg-tertiary)', borderRadius: '6px', textAlign: 'center' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>BGWriter Eff</div>
              <div style={{ fontSize: '1.2rem', fontWeight: 700, color: 'var(--success)' }}>
                {data.bg_writer_eff_pct.toFixed(0)}<span style={{ fontSize: '0.6em' }}>%</span>
              </div>
            </div>
            <div style={{ flex: 1, minWidth: '120px', padding: '0.5rem', background: 'var(--bg-tertiary)', borderRadius: '6px', textAlign: 'center' }}>
              <div style={{ fontSize: '0.7rem', color: 'var(--text-muted)' }}>Cluster State</div>
              <div style={{ fontSize: '1.2rem', fontWeight: 700, color: 'var(--accent-blue)' }}>
                {data.cluster_state}
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
