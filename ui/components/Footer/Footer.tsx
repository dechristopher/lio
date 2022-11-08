import Link from "next/link";
import React from "react";
import styles from "./Footer.module.scss";

export const Footer = () => {
	return (
		<div className="mt-1">
			<div className="flex items-center text-xs font-bold">
				<Link href="/about">
					<div className="cursor-pointer">About</div>
				</Link>
				<div className={styles.dot} />
				<Link href="/db">
					<div className="cursor-pointer">DB</div>
				</Link>
				<div className={styles.dot} />
				<Link href="https://github.com/dechristopher/lio">
					<div className="cursor-pointer">{`v${process.env.NEXT_PUBLIC_APP_VERSION}`}</div>
				</Link>
			</div>
			<p className={styles.copyright}> 2021-2022 lioctad.org</p>
		</div>
	);
};
