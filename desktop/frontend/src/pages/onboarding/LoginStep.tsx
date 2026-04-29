import { Loader2 } from "lucide-react";
import { useTranslation } from "react-i18next";

import { palette, space } from "../../layout/theme";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

import { ConfirmedBanner, Field } from "./common";

interface Props {
    url: string;
    username: string;
    password: string;
    busy: boolean;
    onUsername: (v: string) => void;
    onPassword: (v: string) => void;
    onBack: () => void;
    onSubmit: () => void;
}

export default function LoginStep({
    url,
    username,
    password,
    busy,
    onUsername,
    onPassword,
    onBack,
    onSubmit,
}: Props) {
    const { t } = useTranslation("onboarding");
    const { t: tc } = useTranslation("common");
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <ConfirmedBanner url={url} hint={t("login.title")} />
            <div>
                <h2 style={{ margin: 0, fontSize: 20, fontWeight: 600 }}>
                    {t("login.title")}
                </h2>
                <p
                    style={{
                        margin: `${space[1]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 13,
                    }}
                >
                    {t("login.subtitle")}
                </p>
            </div>
            <Field label={tc("labels.username")}>
                <Input
                    autoFocus
                    value={username}
                    onChange={(e) => onUsername(e.target.value)}
                    data-testid="onboarding-username"
                />
            </Field>
            <Field label={tc("labels.password")}>
                <Input
                    type="password"
                    value={password}
                    onChange={(e) => onPassword(e.target.value)}
                    data-testid="onboarding-password"
                />
            </Field>
            <div style={{ display: "flex", gap: space[2] }}>
                <Button variant="outline" onClick={onBack}>
                    {tc("actions.back")}
                </Button>
                <Button
                    onClick={onSubmit}
                    disabled={busy || !username || !password}
                    style={{ marginLeft: "auto" }}
                    data-testid="onboarding-login"
                >
                    {busy && <Loader2 className="size-3.5 animate-spin" />}
                    {tc("actions.logIn")}
                </Button>
            </div>
        </div>
    );
}
