import Link from "next/link";

export default function Error() {
	return (
		<div className="container text-center">
			<p>Room not found!</p>
			<Link href="/">Go back home</Link>
		</div>
	);
}
