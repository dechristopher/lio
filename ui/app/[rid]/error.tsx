"use client";

import Button from "@client/components/Button/Button";
import { useRouter } from "next/navigation";

// TODO add logging to external service
export default function Error({ error }: { error: Error }) {
	const router = useRouter();

	return (
		<div className="text-center">
			<p>Something went wrong!</p>
			<Button className="mt-2" onClick={() => router.push("/")}>
				Go back home
			</Button>
		</div>
	);
}
