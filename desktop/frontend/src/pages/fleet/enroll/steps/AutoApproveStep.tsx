import Mono from "../../../../components/Mono";
import { palette, radius, space } from "../../../../layout/theme";

interface Props {
    autoApprove: boolean;
    onChange: (v: boolean) => void;
}

export default function AutoApproveStep({ autoApprove, onChange }: Props) {
    return (
        <label
            data-testid="enroll-wizard-auto-approve"
            style={{
                display: "flex",
                alignItems: "flex-start",
                gap: space[2],
                border: `1px solid ${palette.border}`,
                borderRadius: radius.md,
                padding: space[3],
                cursor: "pointer",
            }}
        >
            <input
                type="checkbox"
                checked={autoApprove}
                onChange={(e) => onChange(e.target.checked)}
                style={{ marginTop: 4 }}
            />
            <span style={{ fontSize: 12, lineHeight: 1.5 }}>
                <span style={{ fontWeight: 500 }}>Skip admin approval (automation)</span>
                <br />
                <span style={{ color: palette.textMuted }}>
                    When off (default), the new host lands in <Mono>pending</Mono> and
                    an admin must approve it before opening links.
                </span>
            </span>
        </label>
    );
}
