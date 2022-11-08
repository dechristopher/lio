import React, { ReactNode } from "react";
import { Footer } from "../Footer/Footer";
import { Header } from "../Header/Header";
import styles from "./ContentWrapper.module.scss";

interface ContentWrapperProps {
	children?: ReactNode;
}

const ContentWrapper = (props: ContentWrapperProps) => {
	return (
		<div className="flex flex-col items-center pt-8">
			<Header />
			<div className={styles.body}>{props.children}</div>
			<Footer />
		</div>
	);
};

export default ContentWrapper;
