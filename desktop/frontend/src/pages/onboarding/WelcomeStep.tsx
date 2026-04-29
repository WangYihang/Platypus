import { ArrowRight } from "lucide-react";

import { palette, space } from "../../layout/theme";
import { Button } from "@/components/ui/button";

export default function WelcomeStep({ onNext }: { onNext: () => void }) {
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
                    Welcome to Platypus
                </h1>
                <p
                    style={{
                        margin: `${space[2]}px 0 0`,
                        color: palette.textSecondary,
                        fontSize: 14,
                        lineHeight: 1.6,
                    }}
                >
                    Let's connect you to a server. It takes about thirty seconds.
                    You'll need the server's URL, and either a username/password
                    or, on a fresh install, the first-admin secret from
                    <code> &lt;data-dir&gt;/bootstrap.secret</code> on the server.
                </p>
            </div>
            <Button onClick={onNext} size="lg" data-testid="onboarding-get-started">
                Get started
                <ArrowRight className="size-4" />
            </Button>
        </div>
    );
}
