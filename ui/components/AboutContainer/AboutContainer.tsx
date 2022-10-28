import Link from "next/link";
import React from "react";
import Button from "../Button/Button";
import MainContainer from "../MainContainer/MainContainer";
import { FcWithChildren } from "../shared";
import {
	AboutContainerStyle,
	AboutRedirect,
	AboutButtonGroup,
	AboutButton,
} from "./AboutContainer.css";

const AboutContainer: FcWithChildren = (props) => {
	return (
		<MainContainer>
			<div className={AboutContainerStyle}>
				<Link href="/about">
					<div className={AboutRedirect}>About lioctad.org</div>
				</Link>

				{props.children}

				<div className={AboutButtonGroup}>
					<Link href="/about/board">
						<a>
							<Button className={AboutButton}>Board</Button>
						</a>
					</Link>
					<Link href="/about/rules">
						<a>
							<Button className={AboutButton}>Rules</Button>
						</a>
					</Link>
					<Link href="/about/misc">
						<a>
							<Button className={AboutButton}>Misc.</Button>
						</a>
					</Link>
				</div>
			</div>
		</MainContainer>
	);
};

export default AboutContainer;
