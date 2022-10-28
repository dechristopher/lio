import React, { FC } from "react";
import styles from "./Chin.module.scss";

interface ChinProps {
  latency: number;
}

const Chin: FC<ChinProps> = (props) => {
	const numConnected = 1;

	return (
		<div className={styles.chin}>
			<div className="mr-1">{`${numConnected} CONNECTED`}</div>
			<div>{`(${props.latency}ms)`}</div>
		</div>
	);
};

export default Chin;
