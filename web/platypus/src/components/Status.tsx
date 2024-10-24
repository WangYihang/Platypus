import { useQuery } from "@tanstack/react-query";
import { useState, useEffect } from "react";
import { Status } from "@/types/status";
import {
    Card,
    CardContent,
    CardDescription,
    CardFooter,
    CardHeader,
    CardTitle,
} from "@/components/ui/card"
import { Progress } from "@/components/ui/progress"
import { filesize } from "filesize";
import moment from "moment";

export function StatusPage() {
    const [token, setToken] = useState<string | null>(null);

    useEffect(() => {
        const storedToken = localStorage.getItem("token");
        setToken(storedToken);
    }, []);

    const {
        isPending,
        isError, data, error
    } = useQuery<Status>({
        queryKey: ["status"],
        queryFn: async () => {
            const result = await fetch("/api/v1/status", {
                headers: {
                    Authorization: `Bearer ${token}`,
                },
            });
            return result.json();
        },
        enabled: !!token,
        refetchInterval: 60 * 1000,
    });

    if (isPending) {
        return <div>Loading...</div>;
    }

    if (isError) {
        return <div>Error: {error.message}</div>;
    }

    return (
        <Card className="max-w-md mx-auto">
            <CardHeader>
                <CardTitle>System Status</CardTitle>
                <CardDescription>
                    {data.network.hostname}
                </CardDescription>
            </CardHeader>
            <CardContent>
                {/* OS and Architecture in one row */}
                <div className="flex justify-between">
                    <p><strong>OS:</strong> {data.os.name}</p>
                    <p><strong>Version:</strong> {data.os.version}</p>
                    <p><strong>Arch:</strong> {data.os.arch}</p>
                </div>

                {/* CPU */}
                <div className="mt-4">
                    <p><strong>CPU:</strong> {data.cpu.num_cores} Cores ({data.cpu.percent.toFixed(2)}%)</p>
                    <Progress value={data.cpu.percent} />
                </div>

                {/* Disk */}
                <div className="mt-4">
                    <p><strong>Disk:</strong> {filesize(data.disk.used)} / {filesize(data.disk.total)} ({(data.disk.used / data.disk.total * 100).toFixed(2)}%)</p>
                    <Progress value={(data.disk.used / data.disk.total) * 100} />
                </div>

                {/* Memory */}
                <div className="mt-4">
                    <p><strong>Memory:</strong> {filesize(data.memory.used)} / {filesize(data.memory.total)} ({(data.memory.used / data.memory.total * 100).toFixed(2)}%)</p>
                    <Progress value={(data.memory.used / data.memory.total) * 100} />
                </div>

                {/* Go Runtime */}
                <div className="mt-4 flex justify-between">
                    <p><strong>Go:</strong> {data.go.version}</p>
                    <p><strong>Memory</strong>: {filesize(data.go.memory_usage)}</p>
                    <p><strong>Goroutines:</strong> {data.go.num_goroutines}</p>
                </div>
            </CardContent>
            <CardFooter>
                <p className="text-xs text-gray-500">
                    {moment(data.timestamp).format("LLLL")}
                    <span className="ml-2">({moment(data.timestamp).fromNow()})</span>
                </p>
            </CardFooter>
        </Card>
    );
}
