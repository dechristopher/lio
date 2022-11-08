import Link from "next/link";
import React, { ReactNode } from "react";
import Button from "../Button/Button";
import ContentWrapper from "../ContentWrapper/ContentWrapper";
import styles from "./AboutContainer.module.scss";

interface AboutContainerProps {
	children?: ReactNode;
}

const AboutContainer = (props: AboutContainerProps) => {
	return (
		<ContentWrapper>
			<div className="text-left w-80">
				<Link href="/about">
					<div className={styles.header}>About lioctad.org</div>
				</Link>

				{props.children}

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
		</ContentWrapper>
	);
};

export default AboutContainer;
