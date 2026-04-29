import { Boxes, HelpCircle, Laptop, Layers, Monitor, Server } from "lucide-react";

// Icon registry for `Host.machine_type`. Lives next to HostCard
// because it's the only consumer; if other surfaces ever need the
// same mapping, lift it into lib/icons.ts.
const machineTypeIcons: Record<
    string,
    { label: string; Icon: React.ComponentType<{ className?: string }> }
> = {
    container: { label: "container", Icon: Boxes },
    vm: { label: "virtual machine", Icon: Layers },
    bare_metal: { label: "bare metal", Icon: Server },
    laptop: { label: "laptop", Icon: Laptop },
    desktop: { label: "desktop", Icon: Monitor },
    unknown: { label: "unknown", Icon: HelpCircle },
};

export function MachineTypeIcon({ type }: { type?: string }) {
    const meta = type ? machineTypeIcons[type] : undefined;
    if (!meta) return <HelpCircle className="size-4 text-text-muted" />;
    const { Icon } = meta;
    return <Icon className="size-4 text-text-secondary" />;
}
