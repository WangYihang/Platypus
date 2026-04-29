import { ReactNode } from "react";
import { NavLink, useLocation } from "react-router-dom";

import { icons } from "../lib/icons";
import { SessionUser } from "../lib/auth";
import { palette, space } from "./theme";

interface Tab {
    to: string;
    label: string;
    icon: ReactNode;
    minRole?: SessionUser["role"];
    // matchPaths lets a tab claim "active" for paths nested below it
    // even when react-router's NavLink isActive wouldn't (e.g. Fleet
    // owns /hosts/:id; Audit owns /audit/activities, /audit/recordings,
    // …). Each entry is matched as a strict equal-or-startsWith
    // ("/<path>/") so /fleet does not light up for /fleet-x.
    matchPaths?: string[];
}

interface Props {
    user: SessionUser;
    currentSlug?: string;
}

// NavTabs renders the second row of the top bar. It picks one of two
// tab sets depending on whether a project is in scope:
//
//   project → Overview · Fleet · Members · Audit · Settings
//   global  → Projects
//
// Admin destinations (Users / Access Control / Server settings) are
// reachable from UserMenu's popover — they're not promoted to top
// tabs in this iteration. Account / Preferences also live in the
// UserMenu.
//
// The mapping mirrors today's IA exactly so the chrome migration
// (sidebar → top bar) lands without changing any URLs. Subsequent
// iterations will (a) split Audit into History/Operations and (b)
// promote Servers + Admin to global top tabs.
export default function NavTabs({ user, currentSlug }: Props) {
    const { pathname } = useLocation();
    const I = icons;

    const projectBase = currentSlug ? `/projects/${currentSlug}` : null;

    const projectTabs: Tab[] = projectBase
        ? [
              {
                  to: `${projectBase}/overview`,
                  label: "Overview",
                  icon: <I.project className="size-3.5" />,
              },
              {
                  to: `${projectBase}/fleet`,
                  label: "Fleet",
                  icon: <I.fleet className="size-3.5" />,
                  // Fleet still owns the /hosts/:id deep link until
                  // the Fleet master-detail refactor lands.
                  matchPaths: [`${projectBase}/fleet`, `${projectBase}/hosts`],
              },
              {
                  to: `${projectBase}/members`,
                  label: "Members",
                  icon: <I.members className="size-3.5" />,
                  minRole: "operator",
              },
              {
                  to: `${projectBase}/audit/activities`,
                  label: "Audit",
                  icon: <I.audit className="size-3.5" />,
                  matchPaths: [`${projectBase}/audit`],
              },
              {
                  to: `${projectBase}/settings`,
                  label: "Settings",
                  icon: <I.settings className="size-3.5" />,
                  minRole: "admin",
              },
          ]
        : [];

    const globalTabs: Tab[] = [
        {
            to: "/projects",
            label: "Projects",
            icon: <I.projects className="size-3.5" />,
            matchPaths: ["/projects"],
        },
    ];

    const tabs = projectBase ? projectTabs : globalTabs;
    const visible = tabs.filter((t) => meetsRole(user.role, t.minRole));

    return (
        <nav
            data-testid="nav-tabs"
            style={{
                flexShrink: 0,
                display: "flex",
                alignItems: "center",
                gap: space[1],
                padding: `0 ${space[3]}px`,
                background: palette.rail,
                borderBottom: `1px solid ${palette.border}`,
                overflow: "auto",
            }}
        >
            {visible.map((tab) => {
                const matchedByPath =
                    tab.matchPaths?.some(
                        (p) => pathname === p || pathname.startsWith(`${p}/`),
                    ) ?? false;
                return (
                    <NavLink
                        key={tab.to}
                        to={tab.to}
                        end={!tab.matchPaths}
                        data-testid={`nav-tab-${tab.label.toLowerCase()}`}
                        className={({ isActive }) =>
                            "pl-top-tab" +
                            (isActive || matchedByPath ? " pl-top-tab--active" : "")
                        }
                    >
                        {tab.icon}
                        <span>{tab.label}</span>
                    </NavLink>
                );
            })}
        </nav>
    );
}

function meetsRole(actual: SessionUser["role"], required?: SessionUser["role"]): boolean {
    if (!required) return true;
    const order = { viewer: 0, operator: 1, admin: 2 };
    return order[actual] >= order[required];
}
