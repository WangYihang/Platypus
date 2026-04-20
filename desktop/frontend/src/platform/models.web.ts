// Hand-written mirrors of the structs in wailsjs/go/models.ts. Used only
// when vite is in --mode web; the desktop build still gets the real
// generated models from `wails generate module`.
//
// Keep these shapes in sync with:
//   - desktop/internal/api/types.go  (Session, Listener, TunnelInfo, DispatchResult)
//   - desktop/internal/app/app.go    (ConnectionStatus)
//   - desktop/internal/profile/profile.go (Profile)

export namespace api {
    export class DispatchResult {
        session_hash!: string;
        output!: string;
        error?: string;

        static createFrom(source: Partial<DispatchResult> = {}): DispatchResult {
            return Object.assign(new DispatchResult(), source);
        }

        constructor(source: Partial<DispatchResult> = {}) {
            Object.assign(this, source);
        }
    }

    export class Listener {
        hash!: string;
        host!: string;
        port!: number;
        encrypted!: boolean;
        group_dispatch!: boolean;
        disable_history!: boolean;
        public_ip!: string;
        shell_path!: string;
        timestamp!: string;
        interfaces!: string[];

        static createFrom(source: Partial<Listener> = {}): Listener {
            return Object.assign(new Listener(), source);
        }

        constructor(source: Partial<Listener> = {}) {
            Object.assign(this, source);
        }
    }

    export class Session {
        hash!: string;
        host!: string;
        port!: number;
        alias!: string;
        user!: string;
        os!: string;
        version?: string;
        network_interfaces!: Record<string, string>;
        python2!: string;
        python3!: string;
        timestamp!: string;
        group_dispatch!: boolean;
        encrypted!: boolean;
        tag!: string;

        static createFrom(source: Partial<Session> = {}): Session {
            return Object.assign(new Session(), source);
        }

        constructor(source: Partial<Session> = {}) {
            Object.assign(this, source);
        }
    }

    export class TunnelInfo {
        type!: string;
        address!: string;

        static createFrom(source: Partial<TunnelInfo> = {}): TunnelInfo {
            return Object.assign(new TunnelInfo(), source);
        }

        constructor(source: Partial<TunnelInfo> = {}) {
            Object.assign(this, source);
        }
    }
}

export namespace app {
    export class ConnectionStatus {
        connected!: boolean;
        profileName!: string;
        url!: string;

        static createFrom(source: Partial<ConnectionStatus> = {}): ConnectionStatus {
            return Object.assign(new ConnectionStatus(), source);
        }

        constructor(source: Partial<ConnectionStatus> = {}) {
            Object.assign(this, source);
        }
    }
}

export namespace profile {
    export class Profile {
        name!: string;
        url!: string;

        static createFrom(source: Partial<Profile> = {}): Profile {
            return Object.assign(new Profile(), source);
        }

        constructor(source: Partial<Profile> = {}) {
            Object.assign(this, source);
        }
    }
}
