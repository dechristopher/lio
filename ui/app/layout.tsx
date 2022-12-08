import { Footer } from "@components/Footer/Footer";
import { Header } from "@components/Header/Header";
import styles from "./HomePage.module.scss";
import "../styles/global.scss";
import { PieceTheme } from "@client/components/Piece/Piece";
import classNames from "classnames";

export default function RootLayout({
	children,
}: {
	children: React.ReactNode;
}) {
	return (
		<html>
			<head />
			{/* TODO retrieve from planned user preference system */}
			{/* TODO evaluate green.css theme */}
			<body className={classNames([PieceTheme.CBURNETT])}>
				<div className="flex flex-col items-center pt-8">
					<Header />
					<div className={styles.body}>{children}</div>
					<Footer />
				</div>
			</body>
		</html>
	);
}
