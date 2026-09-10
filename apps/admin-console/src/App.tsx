import React, { useState } from 'react';
import { 
  Monitor, 
  ShieldCheck, 
  Activity, 
  Users, 
  FolderTree, 
  AlertTriangle, 
  FileText, 
  Settings, 
  KeyRound, 
  RefreshCw,
  Power,
  Play,
  CheckCircle,
  XCircle,
  Clock,
  Laptop,
  Copy,
  Check,
  X,
  Plus,
  Radio,
  Tv,
  EyeOff,
  Package,
  FileCheck
} from 'lucide-react';

interface DeviceItem {
  id: string;
  hostname: string;
  friendlyName: string;
  group: string;
  os: string;
  status: 'ONLINE' | 'OFFLINE' | 'PENDING';
  cpu: number;
  ram: number;
  disk: number;
  lastSeen: string;
}

const initialDevices: DeviceItem[] = [
  { id: 'dev-001', hostname: 'EXAM-PC-01', friendlyName: 'Seat 01', group: 'Exam Lab 1', os: 'Windows 11 Pro', status: 'ONLINE', cpu: 18, ram: 42, disk: 35, lastSeen: 'Just now' },
  { id: 'dev-002', hostname: 'EXAM-PC-02', friendlyName: 'Seat 02', group: 'Exam Lab 1', os: 'Windows 11 Pro', status: 'ONLINE', cpu: 22, ram: 46, disk: 38, lastSeen: 'Just now' },
  { id: 'dev-003', hostname: 'EXAM-PC-03', friendlyName: 'Seat 03', group: 'Exam Lab 1', os: 'Windows 11 Pro', status: 'ONLINE', cpu: 89, ram: 92, disk: 40, lastSeen: 'Just now' },
  { id: 'dev-004', hostname: 'LAB-PC-10', friendlyName: 'Dev Station 10', group: 'Training Room', os: 'Windows 10 Enterprise', status: 'OFFLINE', cpu: 0, ram: 0, disk: 55, lastSeen: '2 hours ago' },
  { id: 'dev-005', hostname: 'NEW-ENROLL-PC', friendlyName: 'Student Laptop A', group: 'Unassigned', os: 'Windows 11 Home', status: 'PENDING', cpu: 12, ram: 30, disk: 20, lastSeen: '5 min ago' },
];

const mockAuditLogs = [
  { id: 101, time: '1 min ago', actor: 'admin@acme-academy.org', action: 'session.requested', resource: 'EXAM-PC-01', status: 'SUCCESS' },
  { id: 102, time: '4 min ago', actor: 'admin@acme-academy.org', action: 'device.approved', resource: 'EXAM-PC-02', status: 'SUCCESS' },
  { id: 103, time: '12 min ago', actor: 'SYSTEM', action: 'device.telemetry', resource: 'LAB-PC-10', status: 'SUCCESS' },
  { id: 104, time: '25 min ago', actor: 'admin@acme-academy.org', action: 'device.invitation.created', resource: 'CH-ENROLL-9F81', status: 'SUCCESS' },
  { id: 105, time: '1 hour ago', actor: 'admin@acme-academy.org', action: 'auth.login.success', resource: 'c1f72776...', status: 'SUCCESS' },
];

export default function App() {
  const [activeTab, setActiveTab] = useState<'dashboard' | 'devices' | 'groups' | 'audit'>('dashboard');
  const [selectedGroup, setSelectedGroup] = useState<string>('ALL');
  const [devices, setDevices] = useState<DeviceItem[]>(initialDevices);

  // Enrollment Modal States
  const [showTokenModal, setShowTokenModal] = useState(false);
  const [generatedToken, setGeneratedToken] = useState<string | null>(null);
  const [tokenCopied, setTokenCopied] = useState(false);
  const [maxUses, setMaxUses] = useState(1);
  const [expiryHours, setExpiryHours] = useState(24);
  const [targetGroup, setTargetGroup] = useState('Exam Lab 1');

  // Restart Confirmation Modal State
  const [restartingDevice, setRestartingDevice] = useState<DeviceItem | null>(null);
  const [restartConfirmed, setRestartConfirmed] = useState(false);

  // Remote Desktop Session States
  const [activeRemoteDevice, setActiveRemoteDevice] = useState<DeviceItem | null>(null);
  const [sessionState, setSessionState] = useState<'REQUESTING' | 'CONNECTED' | 'TERMINATED'>('REQUESTING');

  const handleGenerateToken = () => {
    const randomHex = Array.from(crypto.getRandomValues(new Uint8Array(16)))
      .map(b => b.toString(16).padStart(2, '0'))
      .join('')
      .toUpperCase();
    const token = `CH-ENROLL-${randomHex}`;
    setGeneratedToken(token);
  };

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
    setTokenCopied(true);
    setTimeout(() => setTokenCopied(false), 2000);
  };

  const handleApproveDevice = (deviceId: string) => {
    setDevices(prev => prev.map(dev => {
      if (dev.id === deviceId) {
        return { ...dev, status: 'ONLINE', cpu: 15, ram: 38, disk: 25, lastSeen: 'Just now' };
      }
      return dev;
    }));
  };

  const handleStartRemoteSession = (device: DeviceItem) => {
    setActiveRemoteDevice(device);
    setSessionState('REQUESTING');
    setTimeout(() => {
      setSessionState('CONNECTED');
    }, 1200);
  };

  const handleEndRemoteSession = () => {
    setSessionState('TERMINATED');
    setTimeout(() => {
      setActiveRemoteDevice(null);
    }, 300);
  };

  const handleExecuteRestart = () => {
    if (!restartingDevice) return;
    setRestartConfirmed(true);
    setTimeout(() => {
      setDevices(prev => prev.map(d => d.id === restartingDevice.id ? { ...d, cpu: 5, ram: 20, lastSeen: 'Rebooting...' } : d));
      setRestartingDevice(null);
      setRestartConfirmed(false);
    }, 1500);
  };

  const filteredDevices = selectedGroup === 'ALL' 
    ? devices 
    : devices.filter(d => d.group === selectedGroup);

  const onlineCount = devices.filter(d => d.status === 'ONLINE').length;
  const offlineCount = devices.filter(d => d.status === 'OFFLINE').length;
  const pendingCount = devices.filter(d => d.status === 'PENDING').length;
  const activeSessionCount = activeRemoteDevice && sessionState === 'CONNECTED' ? 1 : 0;

  return (
    <div className="flex h-screen bg-slate-950 text-slate-100 font-sans">
      {/* Sidebar */}
      <aside className="w-64 border-r border-slate-800 bg-slate-900/60 flex flex-col">
        <div className="p-5 flex items-center gap-3 border-b border-slate-800">
          <div className="p-2 bg-blue-600 rounded-lg shadow-lg shadow-blue-500/20">
            <ShieldCheck className="w-6 h-6 text-white" />
          </div>
          <div>
            <h1 className="font-bold text-lg leading-none tracking-tight">ControlHub</h1>
            <span className="text-xs text-blue-400 font-medium">B2B Device Console</span>
          </div>
        </div>

        <nav className="flex-1 p-4 space-y-1.5">
          <button 
            onClick={() => setActiveTab('dashboard')}
            className={`w-full flex items-center gap-3 px-3.5 py-2.5 rounded-lg text-sm font-medium transition-colors ${
              activeTab === 'dashboard' ? 'bg-blue-600 text-white font-semibold' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
            }`}
          >
            <Activity className="w-4 h-4" />
            Dashboard
          </button>

          <button 
            onClick={() => setActiveTab('devices')}
            className={`w-full flex items-center gap-3 px-3.5 py-2.5 rounded-lg text-sm font-medium transition-colors ${
              activeTab === 'devices' ? 'bg-blue-600 text-white font-semibold' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
            }`}
          >
            <Monitor className="w-4 h-4" />
            All Devices
            <span className="ml-auto text-xs bg-slate-800 px-2 py-0.5 rounded-full text-slate-300">
              {devices.length}
            </span>
          </button>

          <button 
            onClick={() => setActiveTab('groups')}
            className={`w-full flex items-center gap-3 px-3.5 py-2.5 rounded-lg text-sm font-medium transition-colors ${
              activeTab === 'groups' ? 'bg-blue-600 text-white font-semibold' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
            }`}
          >
            <FolderTree className="w-4 h-4" />
            Lab Groups
          </button>

          <button 
            onClick={() => setActiveTab('audit')}
            className={`w-full flex items-center gap-3 px-3.5 py-2.5 rounded-lg text-sm font-medium transition-colors ${
              activeTab === 'audit' ? 'bg-blue-600 text-white font-semibold' : 'text-slate-400 hover:text-slate-200 hover:bg-slate-800/60'
            }`}
          >
            <FileText className="w-4 h-4" />
            Audit Ledger
          </button>
        </nav>

        <div className="p-4 border-t border-slate-800">
          <div className="flex items-center gap-3 p-2 rounded-lg bg-slate-800/40">
            <div className="w-8 h-8 rounded-full bg-blue-500/20 text-blue-400 flex items-center justify-center font-bold text-xs">
              AA
            </div>
            <div className="overflow-hidden">
              <p className="text-xs font-semibold truncate">Acme IT Academy</p>
              <p className="text-[10px] text-slate-400 truncate">Plan: 50 Workstations</p>
            </div>
          </div>
        </div>
      </aside>

      {/* Main Content Area */}
      <main className="flex-1 flex flex-col overflow-y-auto">
        {/* Top Navbar */}
        <header className="h-16 border-b border-slate-800 px-8 flex items-center justify-between bg-slate-900/30">
          <div className="flex items-center gap-4">
            <h2 className="text-lg font-semibold capitalize">
              {activeTab === 'dashboard' && 'Executive System Dashboard'}
              {activeTab === 'devices' && 'Managed Workstations'}
              {activeTab === 'groups' && 'Laboratory & Classroom Groups'}
              {activeTab === 'audit' && 'Security & Operational Audit Ledger'}
            </h2>
          </div>

          <div className="flex items-center gap-3">
            <button 
              onClick={() => { setShowTokenModal(true); handleGenerateToken(); }}
              className="flex items-center gap-2 px-3.5 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 transition cursor-pointer"
            >
              <KeyRound className="w-3.5 h-3.5" />
              Generate Enrollment Token
            </button>
          </div>
        </header>

        {/* View Port Content */}
        <div className="p-8 space-y-6">
          {/* Main Views */}
          {(activeTab === 'dashboard' || activeTab === 'devices') && (
            <>
              {/* KPI Cards */}
              <div className="grid grid-cols-4 gap-4">
                <div className="p-4 rounded-xl bg-slate-900/70 border border-slate-800 flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-slate-400">Total Enrolled</p>
                    <p className="text-2xl font-bold mt-1">{devices.length}</p>
                  </div>
                  <div className="p-3 bg-blue-500/10 rounded-lg text-blue-400">
                    <Laptop className="w-5 h-5" />
                  </div>
                </div>

                <div className="p-4 rounded-xl bg-slate-900/70 border border-slate-800 flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-slate-400">Online & Healthy</p>
                    <p className="text-2xl font-bold text-emerald-400 mt-1">{onlineCount}</p>
                  </div>
                  <div className="p-3 bg-emerald-500/10 rounded-lg text-emerald-400">
                    <CheckCircle className="w-5 h-5" />
                  </div>
                </div>

                <div className="p-4 rounded-xl bg-slate-900/70 border border-slate-800 flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-slate-400">Offline</p>
                    <p className="text-2xl font-bold text-slate-400 mt-1">{offlineCount}</p>
                  </div>
                  <div className="p-3 bg-slate-500/10 rounded-lg text-slate-400">
                    <XCircle className="w-5 h-5" />
                  </div>
                </div>

                <div className="p-4 rounded-xl bg-slate-900/70 border border-slate-800 flex items-center justify-between">
                  <div>
                    <p className="text-xs font-medium text-slate-400">Pending Approval</p>
                    <p className="text-2xl font-bold text-amber-400 mt-1">{pendingCount}</p>
                  </div>
                  <div className="p-3 bg-amber-500/10 rounded-lg text-amber-400">
                    <Clock className="w-5 h-5" />
                  </div>
                </div>
              </div>

              {/* Secondary KPI Bar */}
              <div className="grid grid-cols-4 gap-4">
                <div className="p-3 rounded-lg bg-slate-900/40 border border-slate-800/80 flex items-center gap-3">
                  <div className="w-2 h-2 rounded-full bg-blue-400 animate-pulse" />
                  <span className="text-xs text-slate-300">Active Remote Sessions: <strong>{activeSessionCount}</strong></span>
                </div>
                <div className="p-3 rounded-lg bg-slate-900/40 border border-slate-800/80 flex items-center gap-3">
                  <AlertTriangle className="w-3.5 h-3.5 text-amber-400" />
                  <span className="text-xs text-slate-300">CPU Alerts: <strong>1</strong></span>
                </div>
                <div className="p-3 rounded-lg bg-slate-900/40 border border-slate-800/80 flex items-center gap-3">
                  <AlertTriangle className="w-3.5 h-3.5 text-amber-400" />
                  <span className="text-xs text-slate-300">RAM Alerts: <strong>1</strong></span>
                </div>
                <div className="p-3 rounded-lg bg-slate-900/40 border border-slate-800/80 flex items-center gap-3">
                  <AlertTriangle className="w-3.5 h-3.5 text-emerald-400" />
                  <span className="text-xs text-slate-300">Security Health: <strong>100%</strong></span>
                </div>
              </div>

              {/* Device Table Card */}
              <div className="rounded-xl border border-slate-800 bg-slate-900/40 overflow-hidden">
                <div className="p-4 border-b border-slate-800 flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <h3 className="font-semibold text-sm">Managed Workstations</h3>
                    <span className="text-xs text-slate-400">({filteredDevices.length} devices)</span>
                  </div>

                  <div className="flex items-center gap-3 text-xs">
                    <span className="text-slate-400">Filter Lab:</span>
                    {['ALL', 'Exam Lab 1', 'Training Room', 'Unassigned'].map((grp) => (
                      <button
                        key={grp}
                        onClick={() => setSelectedGroup(grp)}
                        className={`px-2.5 py-1 rounded-md transition ${
                          selectedGroup === grp 
                            ? 'bg-blue-600 text-white font-medium' 
                            : 'bg-slate-800 text-slate-400 hover:text-white'
                        }`}
                      >
                        {grp}
                      </button>
                    ))}
                  </div>
                </div>

                <table className="w-full text-left text-xs">
                  <thead className="bg-slate-900/80 text-slate-400 uppercase tracking-wider font-semibold border-b border-slate-800">
                    <tr>
                      <th className="px-6 py-3">Device / Hostname</th>
                      <th className="px-6 py-3">Group</th>
                      <th className="px-6 py-3">Status</th>
                      <th className="px-6 py-3">Telemetry (CPU / RAM / Disk)</th>
                      <th className="px-6 py-3">Last Seen</th>
                      <th className="px-6 py-3 text-right">Actions</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800/60">
                    {filteredDevices.map((dev) => (
                      <tr key={dev.id} className="hover:bg-slate-800/30 transition">
                        <td className="px-6 py-4">
                          <div className="font-semibold text-slate-200">{dev.hostname}</div>
                          <div className="text-[11px] text-slate-400">{dev.friendlyName} • {dev.os}</div>
                        </td>
                        <td className="px-6 py-4 text-slate-300">{dev.group}</td>
                        <td className="px-6 py-4">
                          {dev.status === 'ONLINE' && (
                            <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-emerald-500/10 text-emerald-400 border border-emerald-500/20">
                              <span className="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse" />
                              Online
                            </span>
                          )}
                          {dev.status === 'OFFLINE' && (
                            <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-slate-500/10 text-slate-400 border border-slate-500/20">
                              <span className="w-1.5 h-1.5 rounded-full bg-slate-400" />
                              Offline
                            </span>
                          )}
                          {dev.status === 'PENDING' && (
                            <span className="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-[11px] font-medium bg-amber-500/10 text-amber-400 border border-amber-500/20">
                              <Clock className="w-3 h-3" />
                              Pending Approval
                            </span>
                          )}
                        </td>
                        <td className="px-6 py-4">
                          {dev.status === 'ONLINE' ? (
                            <div className="space-y-1 w-44">
                              <div className="flex justify-between text-[10px] text-slate-400">
                                <span>CPU {dev.cpu}%</span>
                                <span>RAM {dev.ram}%</span>
                                <span>Disk {dev.disk}%</span>
                              </div>
                              <div className="w-full bg-slate-800 h-1.5 rounded-full overflow-hidden flex">
                                <div style={{ width: `${dev.cpu}%` }} className={`h-full ${dev.cpu > 80 ? 'bg-red-500' : 'bg-blue-500'}`} />
                              </div>
                            </div>
                          ) : (
                            <span className="text-slate-500 italic">Telemetry inactive</span>
                          )}
                        </td>
                        <td className="px-6 py-4 text-slate-400">{dev.lastSeen}</td>
                        <td className="px-6 py-4 text-right">
                          <div className="inline-flex items-center gap-1.5">
                            {dev.status === 'PENDING' ? (
                              <button 
                                onClick={() => handleApproveDevice(dev.id)}
                                className="px-3 py-1 bg-emerald-600 hover:bg-emerald-500 text-white rounded font-medium text-[11px] transition shadow-sm cursor-pointer"
                              >
                                Approve
                              </button>
                            ) : dev.status === 'ONLINE' ? (
                              <>
                                <button 
                                  onClick={() => handleStartRemoteSession(dev)}
                                  title="Start Authorized Remote Desktop"
                                  className="p-1.5 bg-blue-600/20 hover:bg-blue-600 text-blue-400 hover:text-white rounded transition cursor-pointer"
                                >
                                  <Play className="w-3.5 h-3.5" />
                                </button>
                                <button 
                                  onClick={() => setRestartingDevice(dev)}
                                  title="Safe Restart Command"
                                  className="p-1.5 hover:bg-slate-800 rounded text-slate-300 hover:text-amber-400 transition cursor-pointer"
                                >
                                  <Power className="w-3.5 h-3.5" />
                                </button>
                              </>
                            ) : null}
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </>
          )}

          {/* Lab Groups View */}
          {activeTab === 'groups' && (
            <div className="space-y-4">
              <div className="flex items-center justify-between">
                <div>
                  <h3 className="font-semibold text-sm">Laboratory & Classroom Containers</h3>
                  <p className="text-xs text-slate-400">Manage device groupings, group policy inheritance, and classroom reboot schedules.</p>
                </div>
              </div>

              <div className="grid grid-cols-3 gap-5">
                <div className="p-5 rounded-xl bg-slate-900/60 border border-slate-800 space-y-4">
                  <div className="flex items-center justify-between">
                    <h4 className="font-semibold text-slate-200">Exam Lab 1</h4>
                    <span className="text-[11px] px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20">3 Workstations</span>
                  </div>
                  <p className="text-xs text-slate-400">Primary certification testing workstations with restricted browsing policy.</p>
                  <div className="space-y-1.5 pt-2 border-t border-slate-800 text-xs">
                    <div className="flex justify-between text-slate-400">
                      <span>Online Status:</span>
                      <span className="text-emerald-400 font-medium">3 / 3 Active</span>
                    </div>
                    <div className="flex justify-between text-slate-400">
                      <span>Consent Mode:</span>
                      <span className="text-slate-200">Mandatory</span>
                    </div>
                    <div className="flex justify-between text-slate-400">
                      <span>Average CPU:</span>
                      <span className="text-slate-200">43.0%</span>
                    </div>
                  </div>
                </div>

                <div className="p-5 rounded-xl bg-slate-900/60 border border-slate-800 space-y-4">
                  <div className="flex items-center justify-between">
                    <h4 className="font-semibold text-slate-200">Training Room</h4>
                    <span className="text-[11px] px-2 py-0.5 rounded bg-blue-500/10 text-blue-400 border border-blue-500/20">1 Workstation</span>
                  </div>
                  <p className="text-xs text-slate-400">General software development and IT training lab.</p>
                  <div className="space-y-1.5 pt-2 border-t border-slate-800 text-xs">
                    <div className="flex justify-between text-slate-400">
                      <span>Online Status:</span>
                      <span className="text-slate-400 font-medium">0 / 1 Active</span>
                    </div>
                    <div className="flex justify-between text-slate-400">
                      <span>Consent Mode:</span>
                      <span className="text-slate-200">Optional</span>
                    </div>
                    <div className="flex justify-between text-slate-400">
                      <span>Average CPU:</span>
                      <span className="text-slate-500 italic">Offline</span>
                    </div>
                  </div>
                </div>

                <div className="p-5 rounded-xl bg-slate-900/30 border border-dashed border-slate-800 flex flex-col items-center justify-center text-center p-6 space-y-2">
                  <FolderTree className="w-8 h-8 text-slate-600" />
                  <p className="text-xs font-semibold text-slate-400">Create New Lab Group</p>
                  <p className="text-[10px] text-slate-500">Organize workstations by classroom, lab, or campus location.</p>
                </div>
              </div>
            </div>
          )}

          {/* Audit Log View */}
          {activeTab === 'audit' && (
            <div className="space-y-4">
              <div>
                <h3 className="font-semibold text-sm">Security & Compliance Audit Ledger</h3>
                <p className="text-xs text-slate-400">Immutable record of all administrative actions, logins, remote desktop sessions, and command dispatches.</p>
              </div>

              <div className="rounded-xl border border-slate-800 bg-slate-900/40 overflow-hidden">
                <table className="w-full text-left text-xs">
                  <thead className="bg-slate-900/80 text-slate-400 uppercase tracking-wider font-semibold border-b border-slate-800">
                    <tr>
                      <th className="px-6 py-3">Timestamp</th>
                      <th className="px-6 py-3">Actor / Operator</th>
                      <th className="px-6 py-3">Action Event</th>
                      <th className="px-6 py-3">Target Resource</th>
                      <th className="px-6 py-3 text-right">Status</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-800/60">
                    {mockAuditLogs.map((log) => (
                      <tr key={log.id} className="hover:bg-slate-800/30 transition">
                        <td className="px-6 py-3 text-slate-400 font-mono">{log.time}</td>
                        <td className="px-6 py-3 font-semibold text-slate-200">{log.actor}</td>
                        <td className="px-6 py-3">
                          <span className="font-mono text-blue-400 bg-blue-500/10 px-2 py-0.5 rounded border border-blue-500/20">
                            {log.action}
                          </span>
                        </td>
                        <td className="px-6 py-3 text-slate-300">{log.resource}</td>
                        <td className="px-6 py-3 text-right">
                          <span className="text-emerald-400 font-semibold text-[11px]">SUCCESS</span>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </div>
          )}
        </div>
      </main>

      {/* Safe Restart Confirmation Modal */}
      {restartingDevice && (
        <div className="fixed inset-0 z-50 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-md p-6 space-y-4 shadow-2xl">
            <div className="flex items-center gap-3 text-amber-400">
              <AlertTriangle className="w-5 h-5" />
              <h4 className="font-semibold text-sm text-slate-100">Confirm Workstation Restart</h4>
            </div>

            <p className="text-xs text-slate-300 leading-relaxed">
              You are about to issue a controlled restart job to <strong className="text-white">{restartingDevice.hostname}</strong> ({restartingDevice.friendlyName}). The workstation will receive a signed command and reboot within 60 seconds.
            </p>

            <div className="p-3 bg-slate-950 rounded-lg border border-slate-800 text-[11px] text-slate-400">
              Pipeline: <span className="text-blue-400 font-mono">RBAC Check -> Signed Nonce -> Job Queue -> Audit Event</span>
            </div>

            <div className="flex justify-end gap-2 pt-2 border-t border-slate-800">
              <button 
                onClick={() => setRestartingDevice(null)}
                className="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-300 transition"
              >
                Cancel
              </button>
              <button 
                onClick={handleExecuteRestart}
                disabled={restartConfirmed}
                className="px-4 py-2 rounded-lg bg-amber-600 hover:bg-amber-500 text-xs font-semibold text-white transition shadow-sm flex items-center gap-2"
              >
                {restartConfirmed ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Power className="w-3.5 h-3.5" />}
                {restartConfirmed ? 'Dispatching Job...' : 'Execute Restart'}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Remote Desktop Session Modal */}
      {activeRemoteDevice && (
        <div className="fixed inset-0 z-50 bg-slate-950/90 backdrop-blur-md flex flex-col p-4">
          <div className="flex items-center justify-between bg-slate-900 border border-slate-800 rounded-t-xl px-5 py-3 shadow-lg">
            <div className="flex items-center gap-3">
              <div className="flex items-center gap-2 px-2.5 py-1 rounded bg-cyan-500/10 border border-cyan-500/30 text-cyan-400 font-mono text-xs font-semibold">
                <span className="w-2 h-2 rounded-full bg-cyan-400 animate-ping" />
                WEBRTC DTLS-SRTP ACTIVE
              </div>
              <div className="text-xs text-slate-300">
                Workstation: <strong className="text-white">{activeRemoteDevice.hostname}</strong> ({activeRemoteDevice.friendlyName})
              </div>
              <div className="text-xs text-slate-400 border-l border-slate-700 pl-3">
                Operator: <span className="text-blue-400">admin@acme-academy.org</span>
              </div>
            </div>

            <div className="flex items-center gap-3">
              <span className="text-[11px] text-amber-400 font-medium">
                Mandatory Neon Border & Top-most Consent Active on Endpoint
              </span>
              <button 
                onClick={handleEndRemoteSession}
                className="px-3.5 py-1.5 rounded-lg bg-red-600 hover:bg-red-500 text-white font-semibold text-xs transition shadow-sm cursor-pointer"
              >
                Disconnect Session
              </button>
            </div>
          </div>

          <div className="flex-1 bg-black relative flex items-center justify-center border-4 border-cyan-400 shadow-[0_0_20px_rgba(6,182,212,0.4)] overflow-hidden">
            {sessionState === 'REQUESTING' ? (
              <div className="text-center space-y-3">
                <div className="w-10 h-10 border-2 border-cyan-400 border-t-transparent rounded-full animate-spin mx-auto" />
                <p className="text-sm font-semibold text-cyan-300">Prompting Endpoint for Consent...</p>
                <p className="text-xs text-slate-400">Waiting for user at {activeRemoteDevice.hostname} to click "Allow"</p>
              </div>
            ) : (
              <div className="w-full h-full bg-slate-900/90 flex flex-col">
                <div className="p-4 bg-slate-950/60 border-b border-slate-800 flex items-center justify-between text-xs text-slate-400">
                  <span>Display: Primary 1920x1080 @ 60Hz (DXGI Desktop Duplication)</span>
                  <span>Latency: 18ms | Codec: H.264 / WebRTC</span>
                </div>

                <div className="flex-1 flex flex-col items-center justify-center p-8 space-y-4">
                  <div className="p-6 bg-slate-800/60 border border-slate-700 rounded-2xl max-w-md text-center space-y-2">
                    <Tv className="w-12 h-12 text-cyan-400 mx-auto" />
                    <h4 className="font-semibold text-sm text-slate-100">Live Remote Screen Feed Active</h4>
                    <p className="text-xs text-slate-300 leading-relaxed">
                      You are controlling <span className="text-cyan-300 font-mono">{activeRemoteDevice.hostname}</span>. The endpoint user sees the persistent high-contrast neon screen border and can disconnect anytime.
                    </p>
                  </div>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Enrollment Token Modal */}
      {showTokenModal && (
        <div className="fixed inset-0 z-50 bg-slate-950/80 backdrop-blur-sm flex items-center justify-center p-4">
          <div className="bg-slate-900 border border-slate-800 rounded-2xl w-full max-w-md p-6 space-y-5 shadow-2xl">
            <div className="flex items-center justify-between border-b border-slate-800 pb-3">
              <div className="flex items-center gap-2 text-slate-100 font-semibold text-sm">
                <KeyRound className="w-4 h-4 text-blue-400" />
                Generate Secure Enrollment Token
              </div>
              <button 
                onClick={() => setShowTokenModal(false)}
                className="text-slate-400 hover:text-slate-200 transition"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="space-y-4 text-xs">
              <div>
                <label className="text-slate-400 block mb-1 font-medium">Target Lab / Group</label>
                <select 
                  value={targetGroup}
                  onChange={(e) => setTargetGroup(e.target.value)}
                  className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 outline-none"
                >
                  <option>Exam Lab 1</option>
                  <option>Exam Lab 2</option>
                  <option>Training Room</option>
                  <option>Unassigned</option>
                </select>
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-slate-400 block mb-1 font-medium">Max Uses</label>
                  <input 
                    type="number" 
                    value={maxUses}
                    onChange={(e) => setMaxUses(parseInt(e.target.value) || 1)}
                    min="1"
                    max="100"
                    className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 outline-none"
                  />
                </div>
                <div>
                  <label className="text-slate-400 block mb-1 font-medium">Expires In (Hours)</label>
                  <input 
                    type="number" 
                    value={expiryHours}
                    onChange={(e) => setExpiryHours(parseInt(e.target.value) || 24)}
                    min="1"
                    max="168"
                    className="w-full bg-slate-800 border border-slate-700 rounded-lg px-3 py-2 text-slate-200 outline-none"
                  />
                </div>
              </div>

              {generatedToken && (
                <div className="p-3 bg-slate-950 border border-blue-500/30 rounded-lg space-y-2">
                  <div className="text-[11px] text-slate-400 font-medium">One-Time Token:</div>
                  <div className="flex items-center justify-between bg-slate-900 px-3 py-2 rounded font-mono text-blue-300 text-xs break-all">
                    <span>{generatedToken}</span>
                    <button 
                      onClick={() => copyToClipboard(generatedToken)}
                      className="ml-2 p-1.5 bg-slate-800 hover:bg-slate-700 rounded text-slate-300 transition"
                      title="Copy Token"
                    >
                      {tokenCopied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                    </button>
                  </div>
                  <p className="text-[10px] text-amber-400/90 leading-tight">
                    Enter this token in the ControlHub Windows Agent installer on the target workstation.
                  </p>
                </div>
              )}
            </div>

            <div className="flex justify-end gap-2 pt-2 border-t border-slate-800">
              <button 
                onClick={() => setShowTokenModal(false)}
                className="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-xs font-medium text-slate-300 transition"
              >
                Close
              </button>
              <button 
                onClick={handleGenerateToken}
                className="px-4 py-2 rounded-lg bg-blue-600 hover:bg-blue-500 text-xs font-semibold text-white transition shadow-sm"
              >
                Regenerate Token
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
