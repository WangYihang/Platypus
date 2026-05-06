// InfoActivity is the registry-side wrapper around the Info tab.
// Pulls host / sysInfo / refresh callbacks from HostContext so the
// underlying InfoTab keeps its existing prop shape unchanged.

import InfoTab from "../../InfoTab";
import type { PluginUIProps } from "../registry";
import { useHostContext } from "../../HostContext";

export default function InfoActivity(_props: PluginUIProps) {
    const { host, sysInfo, sysInfoError, sysInfoLoading, refreshSysInfo } =
        useHostContext();
    return (
        <InfoTab
            host={host}
            sysInfo={sysInfo}
            sysInfoError={sysInfoError}
            sysInfoLoading={sysInfoLoading}
            onRefreshSysInfo={refreshSysInfo}
        />
    );
}
