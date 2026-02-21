--
-- PostgreSQL database dump
--

\restrict OHmhDn7bmwpUPlziN2av23gST7ehyZV3UBSF8zzFDaqshmTpgacM5UVIPUN6pgL

-- Dumped from database version 16.11
-- Dumped by pg_dump version 16.11

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

ALTER TABLE IF EXISTS ONLY public.shop_order DROP CONSTRAINT IF EXISTS shop_order_product_id_0eef2166_fk_shop_product_id;
ALTER TABLE IF EXISTS ONLY public.blog_tag_posts DROP CONSTRAINT IF EXISTS blog_tag_posts_tag_id_5f489887_fk_blog_tag_id;
ALTER TABLE IF EXISTS ONLY public.blog_tag_posts DROP CONSTRAINT IF EXISTS blog_tag_posts_post_id_99049a47_fk_blog_post_id;
ALTER TABLE IF EXISTS ONLY public.blog_comment DROP CONSTRAINT IF EXISTS blog_comment_post_id_580e96ef_fk_blog_post_id;
ALTER TABLE IF EXISTS ONLY public.auth_user_user_permissions DROP CONSTRAINT IF EXISTS auth_user_user_permissions_user_id_a95ead1b_fk_auth_user_id;
ALTER TABLE IF EXISTS ONLY public.auth_user_user_permissions DROP CONSTRAINT IF EXISTS auth_user_user_permi_permission_id_1fbb5f2c_fk_auth_perm;
ALTER TABLE IF EXISTS ONLY public.auth_user_groups DROP CONSTRAINT IF EXISTS auth_user_groups_user_id_6a12ed8b_fk_auth_user_id;
ALTER TABLE IF EXISTS ONLY public.auth_user_groups DROP CONSTRAINT IF EXISTS auth_user_groups_group_id_97559544_fk_auth_group_id;
ALTER TABLE IF EXISTS ONLY public.auth_permission DROP CONSTRAINT IF EXISTS auth_permission_content_type_id_2f476e4b_fk_django_co;
ALTER TABLE IF EXISTS ONLY public.auth_group_permissions DROP CONSTRAINT IF EXISTS auth_group_permissions_group_id_b120cbf9_fk_auth_group_id;
ALTER TABLE IF EXISTS ONLY public.auth_group_permissions DROP CONSTRAINT IF EXISTS auth_group_permissio_permission_id_84c5c92e_fk_auth_perm;
DROP INDEX IF EXISTS public.shop_order_product_id_0eef2166;
DROP INDEX IF EXISTS public.blog_tag_posts_tag_id_5f489887;
DROP INDEX IF EXISTS public.blog_tag_posts_post_id_99049a47;
DROP INDEX IF EXISTS public.blog_tag_name_c5718cc8_like;
DROP INDEX IF EXISTS public.blog_comment_post_id_580e96ef;
DROP INDEX IF EXISTS public.auth_user_username_6821ab7c_like;
DROP INDEX IF EXISTS public.auth_user_user_permissions_user_id_a95ead1b;
DROP INDEX IF EXISTS public.auth_user_user_permissions_permission_id_1fbb5f2c;
DROP INDEX IF EXISTS public.auth_user_groups_user_id_6a12ed8b;
DROP INDEX IF EXISTS public.auth_user_groups_group_id_97559544;
DROP INDEX IF EXISTS public.auth_permission_content_type_id_2f476e4b;
DROP INDEX IF EXISTS public.auth_group_permissions_permission_id_84c5c92e;
DROP INDEX IF EXISTS public.auth_group_permissions_group_id_b120cbf9;
DROP INDEX IF EXISTS public.auth_group_name_a6ea08ec_like;
ALTER TABLE IF EXISTS ONLY public.shop_product DROP CONSTRAINT IF EXISTS shop_product_pkey;
ALTER TABLE IF EXISTS ONLY public.shop_order DROP CONSTRAINT IF EXISTS shop_order_pkey;
ALTER TABLE IF EXISTS ONLY public.django_migrations DROP CONSTRAINT IF EXISTS django_migrations_pkey;
ALTER TABLE IF EXISTS ONLY public.django_content_type DROP CONSTRAINT IF EXISTS django_content_type_pkey;
ALTER TABLE IF EXISTS ONLY public.django_content_type DROP CONSTRAINT IF EXISTS django_content_type_app_label_model_76bd3d3b_uniq;
ALTER TABLE IF EXISTS ONLY public.blog_tag_posts DROP CONSTRAINT IF EXISTS blog_tag_posts_tag_id_post_id_3e2e54c9_uniq;
ALTER TABLE IF EXISTS ONLY public.blog_tag_posts DROP CONSTRAINT IF EXISTS blog_tag_posts_pkey;
ALTER TABLE IF EXISTS ONLY public.blog_tag DROP CONSTRAINT IF EXISTS blog_tag_pkey;
ALTER TABLE IF EXISTS ONLY public.blog_tag DROP CONSTRAINT IF EXISTS blog_tag_name_key;
ALTER TABLE IF EXISTS ONLY public.blog_post DROP CONSTRAINT IF EXISTS blog_post_pkey;
ALTER TABLE IF EXISTS ONLY public.blog_comment DROP CONSTRAINT IF EXISTS blog_comment_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_user DROP CONSTRAINT IF EXISTS auth_user_username_key;
ALTER TABLE IF EXISTS ONLY public.auth_user_user_permissions DROP CONSTRAINT IF EXISTS auth_user_user_permissions_user_id_permission_id_14a6b632_uniq;
ALTER TABLE IF EXISTS ONLY public.auth_user_user_permissions DROP CONSTRAINT IF EXISTS auth_user_user_permissions_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_user DROP CONSTRAINT IF EXISTS auth_user_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_user_groups DROP CONSTRAINT IF EXISTS auth_user_groups_user_id_group_id_94350c0c_uniq;
ALTER TABLE IF EXISTS ONLY public.auth_user_groups DROP CONSTRAINT IF EXISTS auth_user_groups_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_permission DROP CONSTRAINT IF EXISTS auth_permission_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_permission DROP CONSTRAINT IF EXISTS auth_permission_content_type_id_codename_01ab375a_uniq;
ALTER TABLE IF EXISTS ONLY public.auth_group DROP CONSTRAINT IF EXISTS auth_group_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_group_permissions DROP CONSTRAINT IF EXISTS auth_group_permissions_pkey;
ALTER TABLE IF EXISTS ONLY public.auth_group_permissions DROP CONSTRAINT IF EXISTS auth_group_permissions_group_id_permission_id_0cd325b0_uniq;
ALTER TABLE IF EXISTS ONLY public.auth_group DROP CONSTRAINT IF EXISTS auth_group_name_key;
DROP TABLE IF EXISTS public.shop_product;
DROP TABLE IF EXISTS public.shop_order;
DROP TABLE IF EXISTS public.django_migrations;
DROP TABLE IF EXISTS public.django_content_type;
DROP TABLE IF EXISTS public.blog_tag_posts;
DROP TABLE IF EXISTS public.blog_tag;
DROP TABLE IF EXISTS public.blog_post;
DROP TABLE IF EXISTS public.blog_comment;
DROP TABLE IF EXISTS public.auth_user_user_permissions;
DROP TABLE IF EXISTS public.auth_user_groups;
DROP TABLE IF EXISTS public.auth_user;
DROP TABLE IF EXISTS public.auth_permission;
DROP TABLE IF EXISTS public.auth_group_permissions;
DROP TABLE IF EXISTS public.auth_group;
SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: auth_group; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.auth_group (
    id integer NOT NULL,
    name character varying(150) NOT NULL
);


ALTER TABLE public.auth_group OWNER TO mabyduck_user;

--
-- Name: auth_group_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.auth_group ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.auth_group_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: auth_group_permissions; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.auth_group_permissions (
    id bigint NOT NULL,
    group_id integer NOT NULL,
    permission_id integer NOT NULL
);


ALTER TABLE public.auth_group_permissions OWNER TO mabyduck_user;

--
-- Name: auth_group_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.auth_group_permissions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.auth_group_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: auth_permission; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.auth_permission (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    content_type_id integer NOT NULL,
    codename character varying(100) NOT NULL
);


ALTER TABLE public.auth_permission OWNER TO mabyduck_user;

--
-- Name: auth_permission_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.auth_permission ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.auth_permission_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: auth_user; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.auth_user (
    id integer NOT NULL,
    password character varying(128) NOT NULL,
    last_login timestamp with time zone,
    is_superuser boolean NOT NULL,
    username character varying(150) NOT NULL,
    first_name character varying(150) NOT NULL,
    last_name character varying(150) NOT NULL,
    email character varying(254) NOT NULL,
    is_staff boolean NOT NULL,
    is_active boolean NOT NULL,
    date_joined timestamp with time zone NOT NULL
);


ALTER TABLE public.auth_user OWNER TO mabyduck_user;

--
-- Name: auth_user_groups; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.auth_user_groups (
    id bigint NOT NULL,
    user_id integer NOT NULL,
    group_id integer NOT NULL
);


ALTER TABLE public.auth_user_groups OWNER TO mabyduck_user;

--
-- Name: auth_user_groups_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.auth_user_groups ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.auth_user_groups_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: auth_user_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.auth_user ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.auth_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: auth_user_user_permissions; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.auth_user_user_permissions (
    id bigint NOT NULL,
    user_id integer NOT NULL,
    permission_id integer NOT NULL
);


ALTER TABLE public.auth_user_user_permissions OWNER TO mabyduck_user;

--
-- Name: auth_user_user_permissions_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.auth_user_user_permissions ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.auth_user_user_permissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: blog_comment; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.blog_comment (
    id bigint NOT NULL,
    author character varying(100) NOT NULL,
    content text NOT NULL,
    post_id bigint NOT NULL
);


ALTER TABLE public.blog_comment OWNER TO mabyduck_user;

--
-- Name: blog_comment_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.blog_comment ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.blog_comment_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: blog_post; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.blog_post (
    id bigint NOT NULL,
    title character varying(200) NOT NULL,
    content text NOT NULL,
    author character varying(100) NOT NULL,
    published_date timestamp with time zone NOT NULL,
    is_published boolean NOT NULL
);


ALTER TABLE public.blog_post OWNER TO mabyduck_user;

--
-- Name: blog_post_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.blog_post ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.blog_post_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: blog_tag; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.blog_tag (
    id bigint NOT NULL,
    name character varying(50) NOT NULL
);


ALTER TABLE public.blog_tag OWNER TO mabyduck_user;

--
-- Name: blog_tag_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.blog_tag ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.blog_tag_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: blog_tag_posts; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.blog_tag_posts (
    id bigint NOT NULL,
    tag_id bigint NOT NULL,
    post_id bigint NOT NULL
);


ALTER TABLE public.blog_tag_posts OWNER TO mabyduck_user;

--
-- Name: blog_tag_posts_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.blog_tag_posts ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.blog_tag_posts_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: django_content_type; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.django_content_type (
    id integer NOT NULL,
    app_label character varying(100) NOT NULL,
    model character varying(100) NOT NULL
);


ALTER TABLE public.django_content_type OWNER TO mabyduck_user;

--
-- Name: django_content_type_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.django_content_type ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.django_content_type_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: django_migrations; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.django_migrations (
    id bigint NOT NULL,
    app character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    applied timestamp with time zone NOT NULL
);


ALTER TABLE public.django_migrations OWNER TO mabyduck_user;

--
-- Name: django_migrations_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.django_migrations ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.django_migrations_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: shop_order; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.shop_order (
    id bigint NOT NULL,
    customer_name character varying(200) NOT NULL,
    order_date timestamp with time zone NOT NULL,
    total numeric(10,2) NOT NULL,
    product_id bigint NOT NULL
);


ALTER TABLE public.shop_order OWNER TO mabyduck_user;

--
-- Name: shop_order_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.shop_order ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.shop_order_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: shop_product; Type: TABLE; Schema: public; Owner: mabyduck_user
--

CREATE TABLE public.shop_product (
    id bigint NOT NULL,
    name character varying(200) NOT NULL,
    description text NOT NULL,
    price numeric(10,2) NOT NULL,
    stock integer NOT NULL
);


ALTER TABLE public.shop_product OWNER TO mabyduck_user;

--
-- Name: shop_product_id_seq; Type: SEQUENCE; Schema: public; Owner: mabyduck_user
--

ALTER TABLE public.shop_product ALTER COLUMN id ADD GENERATED BY DEFAULT AS IDENTITY (
    SEQUENCE NAME public.shop_product_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Data for Name: auth_group; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.auth_group (id, name) FROM stdin;
\.


--
-- Data for Name: auth_group_permissions; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.auth_group_permissions (id, group_id, permission_id) FROM stdin;
\.


--
-- Data for Name: auth_permission; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.auth_permission (id, name, content_type_id, codename) FROM stdin;
1	Can add content type	1	add_contenttype
2	Can change content type	1	change_contenttype
3	Can delete content type	1	delete_contenttype
4	Can view content type	1	view_contenttype
5	Can add permission	2	add_permission
6	Can change permission	2	change_permission
7	Can delete permission	2	delete_permission
8	Can view permission	2	view_permission
9	Can add group	3	add_group
10	Can change group	3	change_group
11	Can delete group	3	delete_group
12	Can view group	3	view_group
13	Can add user	4	add_user
14	Can change user	4	change_user
15	Can delete user	4	delete_user
16	Can view user	4	view_user
17	Can add post	5	add_post
18	Can change post	5	change_post
19	Can delete post	5	delete_post
20	Can view post	5	view_post
21	Can add tag	6	add_tag
22	Can change tag	6	change_tag
23	Can delete tag	6	delete_tag
24	Can view tag	6	view_tag
25	Can add comment	7	add_comment
26	Can change comment	7	change_comment
27	Can delete comment	7	delete_comment
28	Can view comment	7	view_comment
29	Can add product	8	add_product
30	Can change product	8	change_product
31	Can delete product	8	delete_product
32	Can view product	8	view_product
33	Can add order	9	add_order
34	Can change order	9	change_order
35	Can delete order	9	delete_order
36	Can view order	9	view_order
\.


--
-- Data for Name: auth_user; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.auth_user (id, password, last_login, is_superuser, username, first_name, last_name, email, is_staff, is_active, date_joined) FROM stdin;
\.


--
-- Data for Name: auth_user_groups; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.auth_user_groups (id, user_id, group_id) FROM stdin;
\.


--
-- Data for Name: auth_user_user_permissions; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.auth_user_user_permissions (id, user_id, permission_id) FROM stdin;
\.


--
-- Data for Name: blog_comment; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.blog_comment (id, author, content, post_id) FROM stdin;
\.


--
-- Data for Name: blog_post; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.blog_post (id, title, content, author, published_date, is_published) FROM stdin;
1	Test Post	Test content	admin	2025-11-30 17:47:27.830767+00	t
\.


--
-- Data for Name: blog_tag; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.blog_tag (id, name) FROM stdin;
\.


--
-- Data for Name: blog_tag_posts; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.blog_tag_posts (id, tag_id, post_id) FROM stdin;
\.


--
-- Data for Name: django_content_type; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.django_content_type (id, app_label, model) FROM stdin;
1	contenttypes	contenttype
4	auth	user
5	blog	post
6	blog	tag
7	blog	comment
8	shop	product
9	shop	order
2	auth	permission
3	auth	group
\.


--
-- Data for Name: django_migrations; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.django_migrations (id, app, name, applied) FROM stdin;
1	contenttypes	0001_initial	2025-11-30 17:40:13.637915+00
2	contenttypes	0002_remove_content_type_name	2025-11-30 17:40:13.640778+00
3	auth	0001_initial	2025-11-30 17:40:13.669855+00
4	auth	0002_alter_permission_name_max_length	2025-11-30 17:40:13.672199+00
5	auth	0003_alter_user_email_max_length	2025-11-30 17:40:13.674362+00
6	auth	0004_alter_user_username_opts	2025-11-30 17:40:13.676497+00
7	auth	0005_alter_user_last_login_null	2025-11-30 17:40:13.678565+00
8	auth	0006_require_contenttypes_0002	2025-11-30 17:40:13.679517+00
9	auth	0007_alter_validators_add_error_messages	2025-11-30 17:40:13.681488+00
10	auth	0008_alter_user_username_max_length	2025-11-30 17:40:13.685482+00
11	auth	0009_alter_user_last_name_max_length	2025-11-30 17:40:13.687541+00
12	auth	0010_alter_group_name_max_length	2025-11-30 17:40:13.689991+00
13	auth	0011_update_proxy_permissions	2025-11-30 17:40:13.691985+00
14	auth	0012_alter_user_first_name_max_length	2025-11-30 17:40:13.693983+00
15	blog	0001_initial	2025-11-30 17:40:13.70775+00
16	shop	0001_initial	2025-11-30 17:40:13.714603+00
\.


--
-- Data for Name: shop_order; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.shop_order (id, customer_name, order_date, total, product_id) FROM stdin;
\.


--
-- Data for Name: shop_product; Type: TABLE DATA; Schema: public; Owner: mabyduck_user
--

COPY public.shop_product (id, name, description, price, stock) FROM stdin;
1	Test Product	A test product	19.99	10
\.


--
-- Name: auth_group_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.auth_group_id_seq', 1, false);


--
-- Name: auth_group_permissions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.auth_group_permissions_id_seq', 1, false);


--
-- Name: auth_permission_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.auth_permission_id_seq', 36, true);


--
-- Name: auth_user_groups_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.auth_user_groups_id_seq', 1, false);


--
-- Name: auth_user_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.auth_user_id_seq', 1, false);


--
-- Name: auth_user_user_permissions_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.auth_user_user_permissions_id_seq', 1, false);


--
-- Name: blog_comment_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.blog_comment_id_seq', 1, false);


--
-- Name: blog_post_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.blog_post_id_seq', 1, true);


--
-- Name: blog_tag_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.blog_tag_id_seq', 1, false);


--
-- Name: blog_tag_posts_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.blog_tag_posts_id_seq', 1, false);


--
-- Name: django_content_type_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.django_content_type_id_seq', 9, true);


--
-- Name: django_migrations_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.django_migrations_id_seq', 16, true);


--
-- Name: shop_order_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.shop_order_id_seq', 1, false);


--
-- Name: shop_product_id_seq; Type: SEQUENCE SET; Schema: public; Owner: mabyduck_user
--

SELECT pg_catalog.setval('public.shop_product_id_seq', 1, true);


--
-- Name: auth_group auth_group_name_key; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_group
    ADD CONSTRAINT auth_group_name_key UNIQUE (name);


--
-- Name: auth_group_permissions auth_group_permissions_group_id_permission_id_0cd325b0_uniq; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_group_permissions
    ADD CONSTRAINT auth_group_permissions_group_id_permission_id_0cd325b0_uniq UNIQUE (group_id, permission_id);


--
-- Name: auth_group_permissions auth_group_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_group_permissions
    ADD CONSTRAINT auth_group_permissions_pkey PRIMARY KEY (id);


--
-- Name: auth_group auth_group_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_group
    ADD CONSTRAINT auth_group_pkey PRIMARY KEY (id);


--
-- Name: auth_permission auth_permission_content_type_id_codename_01ab375a_uniq; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_permission
    ADD CONSTRAINT auth_permission_content_type_id_codename_01ab375a_uniq UNIQUE (content_type_id, codename);


--
-- Name: auth_permission auth_permission_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_permission
    ADD CONSTRAINT auth_permission_pkey PRIMARY KEY (id);


--
-- Name: auth_user_groups auth_user_groups_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_groups
    ADD CONSTRAINT auth_user_groups_pkey PRIMARY KEY (id);


--
-- Name: auth_user_groups auth_user_groups_user_id_group_id_94350c0c_uniq; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_groups
    ADD CONSTRAINT auth_user_groups_user_id_group_id_94350c0c_uniq UNIQUE (user_id, group_id);


--
-- Name: auth_user auth_user_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user
    ADD CONSTRAINT auth_user_pkey PRIMARY KEY (id);


--
-- Name: auth_user_user_permissions auth_user_user_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_user_permissions
    ADD CONSTRAINT auth_user_user_permissions_pkey PRIMARY KEY (id);


--
-- Name: auth_user_user_permissions auth_user_user_permissions_user_id_permission_id_14a6b632_uniq; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_user_permissions
    ADD CONSTRAINT auth_user_user_permissions_user_id_permission_id_14a6b632_uniq UNIQUE (user_id, permission_id);


--
-- Name: auth_user auth_user_username_key; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user
    ADD CONSTRAINT auth_user_username_key UNIQUE (username);


--
-- Name: blog_comment blog_comment_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_comment
    ADD CONSTRAINT blog_comment_pkey PRIMARY KEY (id);


--
-- Name: blog_post blog_post_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_post
    ADD CONSTRAINT blog_post_pkey PRIMARY KEY (id);


--
-- Name: blog_tag blog_tag_name_key; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_tag
    ADD CONSTRAINT blog_tag_name_key UNIQUE (name);


--
-- Name: blog_tag blog_tag_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_tag
    ADD CONSTRAINT blog_tag_pkey PRIMARY KEY (id);


--
-- Name: blog_tag_posts blog_tag_posts_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_tag_posts
    ADD CONSTRAINT blog_tag_posts_pkey PRIMARY KEY (id);


--
-- Name: blog_tag_posts blog_tag_posts_tag_id_post_id_3e2e54c9_uniq; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_tag_posts
    ADD CONSTRAINT blog_tag_posts_tag_id_post_id_3e2e54c9_uniq UNIQUE (tag_id, post_id);


--
-- Name: django_content_type django_content_type_app_label_model_76bd3d3b_uniq; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.django_content_type
    ADD CONSTRAINT django_content_type_app_label_model_76bd3d3b_uniq UNIQUE (app_label, model);


--
-- Name: django_content_type django_content_type_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.django_content_type
    ADD CONSTRAINT django_content_type_pkey PRIMARY KEY (id);


--
-- Name: django_migrations django_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.django_migrations
    ADD CONSTRAINT django_migrations_pkey PRIMARY KEY (id);


--
-- Name: shop_order shop_order_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.shop_order
    ADD CONSTRAINT shop_order_pkey PRIMARY KEY (id);


--
-- Name: shop_product shop_product_pkey; Type: CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.shop_product
    ADD CONSTRAINT shop_product_pkey PRIMARY KEY (id);


--
-- Name: auth_group_name_a6ea08ec_like; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_group_name_a6ea08ec_like ON public.auth_group USING btree (name varchar_pattern_ops);


--
-- Name: auth_group_permissions_group_id_b120cbf9; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_group_permissions_group_id_b120cbf9 ON public.auth_group_permissions USING btree (group_id);


--
-- Name: auth_group_permissions_permission_id_84c5c92e; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_group_permissions_permission_id_84c5c92e ON public.auth_group_permissions USING btree (permission_id);


--
-- Name: auth_permission_content_type_id_2f476e4b; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_permission_content_type_id_2f476e4b ON public.auth_permission USING btree (content_type_id);


--
-- Name: auth_user_groups_group_id_97559544; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_user_groups_group_id_97559544 ON public.auth_user_groups USING btree (group_id);


--
-- Name: auth_user_groups_user_id_6a12ed8b; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_user_groups_user_id_6a12ed8b ON public.auth_user_groups USING btree (user_id);


--
-- Name: auth_user_user_permissions_permission_id_1fbb5f2c; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_user_user_permissions_permission_id_1fbb5f2c ON public.auth_user_user_permissions USING btree (permission_id);


--
-- Name: auth_user_user_permissions_user_id_a95ead1b; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_user_user_permissions_user_id_a95ead1b ON public.auth_user_user_permissions USING btree (user_id);


--
-- Name: auth_user_username_6821ab7c_like; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX auth_user_username_6821ab7c_like ON public.auth_user USING btree (username varchar_pattern_ops);


--
-- Name: blog_comment_post_id_580e96ef; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX blog_comment_post_id_580e96ef ON public.blog_comment USING btree (post_id);


--
-- Name: blog_tag_name_c5718cc8_like; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX blog_tag_name_c5718cc8_like ON public.blog_tag USING btree (name varchar_pattern_ops);


--
-- Name: blog_tag_posts_post_id_99049a47; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX blog_tag_posts_post_id_99049a47 ON public.blog_tag_posts USING btree (post_id);


--
-- Name: blog_tag_posts_tag_id_5f489887; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX blog_tag_posts_tag_id_5f489887 ON public.blog_tag_posts USING btree (tag_id);


--
-- Name: shop_order_product_id_0eef2166; Type: INDEX; Schema: public; Owner: mabyduck_user
--

CREATE INDEX shop_order_product_id_0eef2166 ON public.shop_order USING btree (product_id);


--
-- Name: auth_group_permissions auth_group_permissio_permission_id_84c5c92e_fk_auth_perm; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_group_permissions
    ADD CONSTRAINT auth_group_permissio_permission_id_84c5c92e_fk_auth_perm FOREIGN KEY (permission_id) REFERENCES public.auth_permission(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: auth_group_permissions auth_group_permissions_group_id_b120cbf9_fk_auth_group_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_group_permissions
    ADD CONSTRAINT auth_group_permissions_group_id_b120cbf9_fk_auth_group_id FOREIGN KEY (group_id) REFERENCES public.auth_group(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: auth_permission auth_permission_content_type_id_2f476e4b_fk_django_co; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_permission
    ADD CONSTRAINT auth_permission_content_type_id_2f476e4b_fk_django_co FOREIGN KEY (content_type_id) REFERENCES public.django_content_type(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: auth_user_groups auth_user_groups_group_id_97559544_fk_auth_group_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_groups
    ADD CONSTRAINT auth_user_groups_group_id_97559544_fk_auth_group_id FOREIGN KEY (group_id) REFERENCES public.auth_group(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: auth_user_groups auth_user_groups_user_id_6a12ed8b_fk_auth_user_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_groups
    ADD CONSTRAINT auth_user_groups_user_id_6a12ed8b_fk_auth_user_id FOREIGN KEY (user_id) REFERENCES public.auth_user(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: auth_user_user_permissions auth_user_user_permi_permission_id_1fbb5f2c_fk_auth_perm; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_user_permissions
    ADD CONSTRAINT auth_user_user_permi_permission_id_1fbb5f2c_fk_auth_perm FOREIGN KEY (permission_id) REFERENCES public.auth_permission(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: auth_user_user_permissions auth_user_user_permissions_user_id_a95ead1b_fk_auth_user_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.auth_user_user_permissions
    ADD CONSTRAINT auth_user_user_permissions_user_id_a95ead1b_fk_auth_user_id FOREIGN KEY (user_id) REFERENCES public.auth_user(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: blog_comment blog_comment_post_id_580e96ef_fk_blog_post_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_comment
    ADD CONSTRAINT blog_comment_post_id_580e96ef_fk_blog_post_id FOREIGN KEY (post_id) REFERENCES public.blog_post(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: blog_tag_posts blog_tag_posts_post_id_99049a47_fk_blog_post_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_tag_posts
    ADD CONSTRAINT blog_tag_posts_post_id_99049a47_fk_blog_post_id FOREIGN KEY (post_id) REFERENCES public.blog_post(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: blog_tag_posts blog_tag_posts_tag_id_5f489887_fk_blog_tag_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.blog_tag_posts
    ADD CONSTRAINT blog_tag_posts_tag_id_5f489887_fk_blog_tag_id FOREIGN KEY (tag_id) REFERENCES public.blog_tag(id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: shop_order shop_order_product_id_0eef2166_fk_shop_product_id; Type: FK CONSTRAINT; Schema: public; Owner: mabyduck_user
--

ALTER TABLE ONLY public.shop_order
    ADD CONSTRAINT shop_order_product_id_0eef2166_fk_shop_product_id FOREIGN KEY (product_id) REFERENCES public.shop_product(id) DEFERRABLE INITIALLY DEFERRED;


--
-- PostgreSQL database dump complete
--

\unrestrict OHmhDn7bmwpUPlziN2av23gST7ehyZV3UBSF8zzFDaqshmTpgacM5UVIPUN6pgL

