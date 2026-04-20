// Web-mode drop-in for wailsjs/go/app/App. Pages import the same names
// from the same relative path; vite.config.ts aliases them here when
// mode=web. Every exported name mirrors the Wails-generated binding.
//
// This file is the W1 skeleton — functions are stubs that throw. W2
// fills in REST+WS implementations.

import type { api, app, profile } from "./models.web";

function notImplemented(name: string): never {
    throw new Error(`App.web.${name}: not implemented yet (W2)`);
}

// Profile / connection ------------------------------------------------------
export function ListProfiles(): Promise<profile.Profile[]> {
    return notImplemented("ListProfiles");
}
export function AddProfile(_name: string, _url: string, _secret: string): Promise<void> {
    return notImplemented("AddProfile");
}
export function RemoveProfile(_name: string): Promise<void> {
    return notImplemented("RemoveProfile");
}
export function Connect(_name: string): Promise<void> {
    return notImplemented("Connect");
}
export function Disconnect(): Promise<void> {
    return notImplemented("Disconnect");
}
export function ConnectionStatus(): Promise<app.ConnectionStatus> {
    return notImplemented("ConnectionStatus");
}

// Sessions ------------------------------------------------------------------
export function ListSessions(): Promise<api.Session[]> {
    return notImplemented("ListSessions");
}
export function SetGroupDispatch(_hash: string, _enabled: boolean): Promise<void> {
    return notImplemented("SetGroupDispatch");
}
export function DispatchCommand(_cmd: string, _timeoutSec: number): Promise<api.DispatchResult[]> {
    return notImplemented("DispatchCommand");
}

// Listeners -----------------------------------------------------------------
export function ListListeners(): Promise<api.Listener[]> {
    return notImplemented("ListListeners");
}
export function CreateListener(_host: string, _port: number, _encrypted: boolean): Promise<void> {
    return notImplemented("CreateListener");
}
export function DeleteListener(_hash: string): Promise<void> {
    return notImplemented("DeleteListener");
}
export function GenerateRaasOneliner(_listenerHash: string, _lang: string): Promise<string> {
    return notImplemented("GenerateRaasOneliner");
}
export function AvailableRaasLanguages(): Promise<string[]> {
    return notImplemented("AvailableRaasLanguages");
}

// Upgrade -------------------------------------------------------------------
export function UpgradeToTermite(_plainHash: string, _targetListenerHash: string): Promise<void> {
    return notImplemented("UpgradeToTermite");
}

// Files ---------------------------------------------------------------------
export function FileSize(_hash: string, _path: string): Promise<number> {
    return notImplemented("FileSize");
}
export function ReadFile(_hash: string, _path: string, _offset: number, _size: number): Promise<number[]> {
    return notImplemented("ReadFile");
}
export function WriteFile(_hash: string, _path: string, _data: number[], _append: boolean): Promise<void> {
    return notImplemented("WriteFile");
}
export function DownloadFile(_hash: string, _remote: string, _local: string): Promise<void> {
    return notImplemented("DownloadFile");
}
export function UploadFile(_hash: string, _remote: string, _local: string): Promise<void> {
    return notImplemented("UploadFile");
}
export function PickFileToUpload(): Promise<string> {
    return notImplemented("PickFileToUpload");
}
export function PickSaveLocation(_defaultName: string): Promise<string> {
    return notImplemented("PickSaveLocation");
}

// Tunnels -------------------------------------------------------------------
export function ListTunnels(_hash: string): Promise<api.TunnelInfo[]> {
    return notImplemented("ListTunnels");
}
export function CreateTunnel(_hash: string, _mode: string, _src: string, _dst: string): Promise<void> {
    return notImplemented("CreateTunnel");
}

// Terminal ------------------------------------------------------------------
export function OpenTerminal(_sessionHash: string): Promise<string> {
    return notImplemented("OpenTerminal");
}
export function SendTerminalInput(_termID: string, _data: number[]): Promise<void> {
    return notImplemented("SendTerminalInput");
}
export function ResizeTerminal(_termID: string, _cols: number, _rows: number): Promise<void> {
    return notImplemented("ResizeTerminal");
}
export function CloseTerminal(_termID: string): Promise<void> {
    return notImplemented("CloseTerminal");
}
