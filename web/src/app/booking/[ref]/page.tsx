import type { Metadata } from "next";
import { StatusView } from "./status";

export const metadata: Metadata = { title: "Status pendaftaran", robots: { index: false } };

export default async function BookingStatusPage({ params }: { params: Promise<{ ref: string }> }) {
  const { ref } = await params;
  return <StatusView reference={ref} />;
}
