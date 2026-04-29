import { ArrowRight } from "lucide-react";
import { useTranslation } from "react-i18next";

import { palette, space } from "../../layout/theme";
import { Button } from "@/components/ui/button";

export default function WelcomeStep({ onNext }: { onNext: () => void }) {
    const { t } = useTranslation("onboarding");
    return (
        <div style={{ display: "flex", flexDirection: "column", gap: space[4] }}>
            <div>
                <h1
                    style={{
                        margin: 0,
                        fontSize: 28,
                        fontWeight: 600,
                        letterSpacing: -0.3,
                    }}
                >
                    {t("welcome.title")}
                </h1>
                <p
                    style={{
                        margin: `${space[2]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 14,
                        lineHeight: 1.6,
                    }}
                >
                    {t("welcome.subtitle")}
                </p>
            </div>
            <Button onClick={onNext} size="lg" data-testid="onboarding-get-started">
                {t("welcome.getStarted")}
                <ArrowRight className="size-4" />
            </Button>
        </div>
    );
}
