import Link from "next/link";
import React from "react";
import Dot from "../Dot/Dot";
import { FooterLinks, FooterCopyRight } from "./Footer.css";

export const Footer = () => {
	return (
		<div>
			<div className="flex justify-between items-center">
				<Link href="/about">
					<div className={FooterLinks}>About</div>
				</Link>
				<Dot />
				<Link href="/db">
					<div className={FooterLinks}>DB</div>
				</Link>
				<Dot />
				<Link href="https://github.com/dechristopher/lio">
					<div
						className={FooterLinks}
					>{`v${process.env.NEXT_PUBLIC_APP_VERSION}`}</div>
				</Link>
			</div>
			<p className={FooterCopyRight}>&copy; 2021-2022 lioctad.org</p>
		</div>
	);
};
