import { useState } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";

interface DataPoint {
  time: string;
  watt: number | null;
}

type ChartPoint = { time: string; watt?: number | null };

const WINDOW_SIZE = 300;

interface PowerChartProps {
  data: DataPoint[];
  height?: number;
}

export function PowerChart({ data, height = 200 }: PowerChartProps) {
  const [yMax, setYMax] = useState(0);

  const dataMax = Math.max(0, ...data.map((d) => d.watt ?? 0));
  if (dataMax > yMax) {
    setYMax(dataMax);
  }

  const chartData: ChartPoint[] =
    data.length >= WINDOW_SIZE
      ? data
      : [
          ...Array.from({ length: WINDOW_SIZE - data.length }, (): ChartPoint => ({
            time: "",
          })),
          ...data,
        ];

  return (
    <ResponsiveContainer width="100%" height={height}>
      <LineChart data={chartData}>
        <CartesianGrid strokeDasharray="3 3" stroke="#333" />
        <XAxis dataKey="time" stroke="#888" fontSize={12} />
        <YAxis stroke="#888" fontSize={12} domain={[0, yMax || "auto"]} />
        <Tooltip
          contentStyle={{ backgroundColor: "#1a1a1a", border: "1px solid #333" }}
        />
        <Line
          type="monotone"
          dataKey="watt"
          stroke="#22c55e"
          strokeWidth={2}
          dot={false}
        />
      </LineChart>
    </ResponsiveContainer>
  );
}
