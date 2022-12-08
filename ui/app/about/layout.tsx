import Button from "@components/Button/Button";
import Link from "next/link";
import styles from "./About.module.scss";

export default function AboutLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<div className="text-left w-80">
			<Link href="/about">
				<div className={styles.header}>About lioctad.org</div>
			</Link>

			{children}

			<div className="flex justify-between mx-3">
				<Link href="/about/board">
					<Button className={styles.aboutBtn}>Board</Button>
				</Link>
				<Link href="/about/rules">
					<Button className={styles.aboutBtn}>Rules</Button>
				</Link>
				<Link href="/about/misc">
					<Button className={styles.aboutBtn}>Misc.</Button>
				</Link>
			</div>
		</div>
	);
}
