import { useSearchParams } from "react-router-dom";

// useEnrollWizardOpen reads / writes the boolean `?enroll=1` flag on
// the current URL. Centralising it means every entry point (Fleet
// header button, inline tile, Command Palette) and the wizard
// component itself agree on a single wire format — and deep-links
// like `…/fleet?enroll=1` land directly in the wizard.
//
// `setOpen(false, { key: "await", value: "enroll" })` is the
// convenience used after the operator finishes the wizard: it strips
// the enroll flag AND arms the existing EnrollmentWaitBanner in one
// history entry so the navigation feels atomic.
export function useEnrollWizardOpen(): {
    open: boolean;
    setOpen: (next: boolean, extraParam?: { key: string; value: string }) => void;
} {
    const [params, setParams] = useSearchParams();
    const open = params.get("enroll") === "1";

    function setOpen(
        next: boolean,
        extraParam?: { key: string; value: string },
    ) {
        const out = new URLSearchParams(params);
        if (next) {
            out.set("enroll", "1");
        } else {
            out.delete("enroll");
        }
        if (extraParam) out.set(extraParam.key, extraParam.value);
        setParams(out, { replace: true });
    }

    return { open, setOpen };
}
