import { Loader2 } from "lucide-react";

import CopyButton from "../../components/CopyButton";
import { palette, radius, space } from "../../layout/theme";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import { ConfirmedBanner, Field } from "./common";

interface Props {
    url: string;
    secret: string;
    username: string;
    password: string;
    busy: boolean;
    onSecret: (v: string) => void;
    onUsername: (v: string) => void;
    onPassword: (v: string) => void;
    onBack: () => void;
    onSubmit: () => void;
}

export default function BootstrapStep({
    url,
    secret,
    username,
    password,
    busy,
    onSecret,
    onUsername,
    onPassword,
    onBack,
    onSubmit,
}: Props) {
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <ConfirmedBanner url={url} hint="Bootstrap admin" />
            <div>
                <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>
                    Create the first admin
                </h2>
                <p
                    style={{
                        margin: `${space[1]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 13,
                    }}
                >
                    Paste the first-admin secret from the server and pick admin credentials.
                </p>
            </div>
            <Field
                label="Server secret"
                helper={
                    <>
                        Look for <code>bootstrap.secret</code> in the server's data
                        directory (mode 0600, written on first boot). On Docker compose,
                        run the command below — copy it to clipboard with the button on
                        the right. You can delete the file once the first admin is created.
                        <div
                            style={{
                                display: "flex",
                                alignItems: "center",
                                gap: space[2],
                                marginTop: space[2],
                            }}
                        >
                            <code
                                style={{
                                    flex: 1,
                                    minWidth: 0,
                                    overflow: "auto",
                                    whiteSpace: "nowrap",
                                    background: palette.surface,
                                    border: `1px solid ${palette.border}`,
                                    borderRadius: radius.sm,
                                    padding: `4px ${space[2]}px`,
                                    fontSize: 12,
                                }}
                            >
                                docker compose exec platypus-server cat /app/data/bootstrap.secret
                            </code>
                            <CopyButton
                                text="docker compose exec platypus-server cat /app/data/bootstrap.secret"
                                label=""
                                successMessage="Command copied"
                            />
                        </div>
                    </>
                }
            >
                <Input
                    autoFocus
                    type="password"
                    value={secret}
                    onChange={(e) => onSecret(e.target.value)}
                    data-testid="onboarding-secret"
                />
            </Field>
            <Field label="Admin username">
                <Input
                    value={username}
                    onChange={(e) => onUsername(e.target.value)}
                />
            </Field>
            <Field label="Admin password">
                <Input
                    type="password"
                    value={password}
                    onChange={(e) => onPassword(e.target.value)}
                />
            </Field>
            <div style={{ display: "flex", gap: space[2] }}>
                <Button variant="outline" onClick={onBack}>
                    Back
                </Button>
                <Button
                    onClick={onSubmit}
                    disabled={busy || !secret || !username || !password}
                    style={{ marginLeft: "auto" }}
                >
                    {busy && <Loader2 className="size-3.5 animate-spin" />}
                    Create admin
                </Button>
            </div>
        </div>
    );
}
