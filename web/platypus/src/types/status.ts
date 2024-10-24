export interface Status {
    os: {
        name: string;
        version: string;
        arch: string;
    };
    network: {
        ip: string;
        hostname: string;
    };
    cpu: {
        percent: number;
        num_cores: number;
        num_threads: number;
    };
    disk: {
        total: number;
        used: number;
    };
    memory: {
        total: number;
        used: number;
    };
    go: {
        num_goroutines: number;
        num_cgo_calls: number;
        version: string;
        memory_usage: number;
    };
    timestamp: string;
}